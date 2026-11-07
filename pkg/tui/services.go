package tui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// composeHolder: what the
// Info tabs, which tick off the loop, read of the newest services refresh.
type composeHolder struct {
	instances atomic.Pointer[presentation.StackInstances]
	project   atomic.Pointer[commands.ComposeProject]
}

func (h *composeHolder) load() (*presentation.StackInstances, *commands.ComposeProject) {
	return h.instances.Load(), h.project.Load()
}

// stackIdentity is what the services panel's contents depend on: which
// directory on which remote, and the project its compose file names.
func stackIdentity(stack *commands.ComposeStack) string {
	if stack == nil {
		return ""
	}

	return stack.Ref() + "\x00" + stack.Name
}

// followStack points Services at the stack Stacks has selected, nil for
// none. A different stack empties the panel and turns away any fetch
// still out for the old one.
func (m *Model) followStack(stack *commands.ComposeStack) {
	previous := m.selectedStack
	m.selectedStack = stack

	if stackIdentity(previous) == stackIdentity(stack) {
		return
	}

	m.refreshes.services.invalidate()
	m.composeState.instances.Store(nil)
	m.composeState.project.Store(nil)

	p := m.panelNamed(servicesView)
	p.empty, m.servicesNote = m.tr.NoServices, ""
	if stack == nil {
		p.empty = m.tr.NoStackSelected
	}

	p.selected = 0
	m.refill(p, func(int) string { return "" }, func() [][]string {
		m.serviceRows.items = nil

		return nil
	})
	m.queue(refresh(m.fetchServices(nil)))
	m.followStackBackups()
}

// fetchServices lists the services of the stack Stacks has selected.
func (m *Model) fetchServices(l *commands.Listing) fetch {
	if m.composeUnavailable() {
		return func() (func(), error) { return func() {}, nil }
	}

	ticket := m.refreshes.services.issue()
	stack, src := m.selectedStack, m.sources()

	if stack == nil || stack.Name == "" {
		return func() (func(), error) {
			return func() {
				if m.refreshes.services.admit(ticket) {
					m.serviceRows.set(m, nil)
				}
			}, nil
		}
	}

	return func() (func(), error) {
		command, err := src.commandFor(stack.Remote)
		if err == nil && !src.onSessionRemote(stack.Remote) {
			err = src.remotes.remoteStatusErr(stack.Remote)
		}

		var (
			services []*commands.ComposeService
			orphans  []*commands.Instance
		)

		if err == nil {
			services, orphans, err = command.GetComposeServices(stack, l)
		}

		if err != nil {
			if src.onSessionRemote(stack.Remote) {
				return nil, err
			}

			note := fmt.Sprintf(src.tr.CannotReachRemote, stack.Remote)
			if errors.Is(err, errConnecting) {
				err = fmt.Errorf(src.tr.ConnectingTo, stack.Remote)
				note = err.Error()
			}

			// Another remote's silence is its stack's row's to report, not a popup on every poll.
			return func() { m.showServicesUnreachable(ticket, err, note) }, nil
		}

		// The project's own config backs the Info tabs' healthcheck line.
		project, projectErr := command.GetComposeProject(stack.Name)
		if projectErr != nil {
			src.incus.Log.Warn().Err(projectErr).Send()
		}

		return func() {
			if !m.refreshes.services.admit(ticket) {
				return
			}

			// A nil project is an answer - no longer compose-managed - where an error isn't one.
			if projectErr == nil {
				m.composeState.project.Store(project)
			}

			m.composeState.instances.Store(&presentation.StackInstances{Remote: stack.Remote, Project: stack.Name, Services: services, Orphans: orphans})
			m.panelNamed(servicesView).empty = m.tr.NoServices
			m.servicesNote = ""
			m.serviceRows.set(m, commands.ServiceRows(services))
			m.setSnapshots()
		}, nil
	}
}

// showServicesUnreachable empties Services for a stack whose remote didn't
// answer, with a note in the list and why in main.
func (m *Model) showServicesUnreachable(ticket uint64, err error, note string) {
	if !m.refreshes.services.admit(ticket) {
		return
	}

	m.composeState.instances.Store(nil)
	m.servicesNote = note
	m.panelNamed(servicesView).empty = err.Error()
	m.serviceRows.set(m, nil)
}

// newServiceRows: Services' fetch is its own, as it reads the selected stack's remote.
func (m *Model) newServiceRows(p *panel) list[*commands.ServiceRow] {
	return list[*commands.ServiceRow]{
		panel:   p,
		less:    sortServiceRows,
		key:     (*commands.ServiceRow).Key,
		project: func(row *commands.ServiceRow) string { return row.Service.Project },
		cells: func(row *commands.ServiceRow, _ bool) []string {
			return presentation.GetServiceRowDisplayStrings(&m.cfg.UserConfig.Gui, row, m.upstreams)
		},
		version: m.serviceVersion,
	}
}

// sortServiceRows: by name, a service's replicas after it, in order.
func sortServiceRows(a, b *commands.ServiceRow) bool {
	if a.Service.Name != b.Service.Name {
		return a.Service.Name < b.Service.Name
	}

	if (a.Instance == nil) != (b.Instance == nil) {
		return a.Instance == nil
	}

	return a.Instance != nil && a.Instance.Name < b.Instance.Name
}

func (m *Model) serviceFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    presentation.ServiceImageColumn(&m.cfg.UserConfig.Gui),
		MinWidth: presentation.MinImageAliasWidth,
	}}
}

func (m *Model) serviceVersion(row *commands.ServiceRow) string {
	// Each refresh builds new ComposeService values, so the instance count
	// is part of the key: a replica coming or going must restart the tab.
	key := row.Service.Status() + "-" + strconv.Itoa(len(row.Service.Instances))

	if row.Instance != nil {
		key += "-" + row.Instance.Instance.Status
	}

	// The config of every instance the row stands for, so the tabs follow a change to one.
	for _, instance := range row.Instances() {
		key += "-" + instance.ConfigFingerprint()
	}

	return key + "-" + presentation.UpstreamVerdict(commands.MostTelling(m.upstreams, presentation.BaseImages(row.Instances())...), m.tr)
}

// servicesTitle names the compose project the panel acts on.
func (m *Model) servicesTitle() string {
	stack := m.selectedStack
	if stack == nil || stack.Name == "" {
		return m.tr.ServicesTitle
	}

	return fmt.Sprintf(m.tr.ServicesTitleProject, m.onRemote(stack.Name, stack.Remote))
}

// followServices points Snapshots at the selected row's instances: a
// service's own row means every replica of it.
func (m *Model) followServices() {
	row := m.serviceRows.selected()
	if row == nil {
		return
	}

	label := row.Service.Name
	if row.Instance != nil {
		label = row.Instance.Name
	}

	label = m.onRemote(label, row.Service.Remote)
	instances := row.Instances()
	following := m.snapshotsFollow

	if following.label == label && len(following.instances) == len(instances) &&
		!slices.ContainsFunc(instances, func(instance *commands.Instance) bool {
			return !slices.ContainsFunc(following.instances, func(other *commands.Instance) bool {
				return other.Key() == instance.Key() && other.Remote == instance.Remote
			})
		}) {
		return
	}

	m.followSnapshotsOf(snapshotsFollow{label: label, instances: instances})
}
