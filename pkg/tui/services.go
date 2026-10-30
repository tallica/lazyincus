package tui

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// composeHolder is pkg/gui's composeInstances and composeProject: what the
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

	p := m.panelNamed("services")
	p.empty, m.servicesNote = m.tr.NoServices, ""
	if stack == nil {
		p.empty = m.tr.NoStackSelected
	}

	p.selected = 0
	m.refill(p, func(int) string { return "" }, func() [][]string {
		m.serviceRows = nil

		return nil
	})
	m.queue(refresh(m.fetchServices()))
}

// fetchServices lists the services of the stack Stacks has selected.
func (m *Model) fetchServices() fetch {
	if m.composeUnavailable() {
		return func() (func(), error) { return func() {}, nil }
	}

	ticket := m.refreshes.services.issue()
	stack := m.selectedStack

	if stack == nil || stack.Name == "" {
		return func() (func(), error) {
			return func() {
				if m.refreshes.services.admit(ticket) {
					m.setServices(nil)
				}
			}, nil
		}
	}

	return func() (func(), error) {
		command, err := m.commandFor(stack.Remote)
		if err == nil && !m.onSessionRemote(stack.Remote) {
			err = m.remotes.remoteStatusErr(stack.Remote)
		}

		var (
			services []*commands.ComposeService
			orphans  []*commands.Instance
		)

		if err == nil {
			services, orphans, err = command.GetComposeServices(stack)
		}

		if err != nil {
			if m.onSessionRemote(stack.Remote) {
				return nil, err
			}

			note := fmt.Sprintf(m.tr.CannotReachRemote, stack.Remote)
			if errors.Is(err, errConnecting) {
				err = fmt.Errorf(m.tr.ConnectingTo, stack.Remote)
				note = err.Error()
			}

			// Another remote's silence is its stack's row's to report, not a popup on every poll.
			return m.showServicesUnreachable(ticket, err, note), nil
		}

		// The project's own config backs the Info tabs' healthcheck line.
		project, projectErr := command.GetComposeProject(stack.Name)
		if projectErr != nil {
			m.incus.Log.Warn().Err(projectErr).Send()
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
			m.panelNamed("services").empty = m.tr.NoServices
			m.servicesNote = ""
			m.setServices(commands.ServiceRows(services))
			m.setSnapshots()
		}, nil
	}
}

// showServicesUnreachable empties Services for a stack whose remote didn't
// answer, with a note in the list and why in main.
func (m *Model) showServicesUnreachable(ticket uint64, err error, note string) func() {
	return func() {
		if !m.refreshes.services.admit(ticket) {
			return
		}

		m.composeState.instances.Store(nil)
		m.servicesNote = note
		m.panelNamed("services").empty = err.Error()
		m.setServices(nil)
	}
}

func (m *Model) setServices(rows []*commands.ServiceRow) {
	m.refill(m.panelNamed("services"), func(item int) string { return m.serviceRows[item].Key() }, func() [][]string {
		sort.SliceStable(rows, func(a, b int) bool { return sortServiceRows(rows[a], rows[b]) })
		m.serviceRows = rows

		return m.serviceCells()
	})
}

// rerenderServices shows the rows' marks, which their status reads.
func (m *Model) rerenderServices() {
	m.refill(m.panelNamed("services"), func(item int) string { return m.serviceRows[item].Key() }, m.serviceCells)
}

// sortServiceRows is pkg/gui's: by name, a service's replicas after it, in order.
func sortServiceRows(a, b *commands.ServiceRow) bool {
	if a.Service.Name != b.Service.Name {
		return a.Service.Name < b.Service.Name
	}

	if (a.Instance == nil) != (b.Instance == nil) {
		return a.Instance == nil
	}

	return a.Instance != nil && a.Instance.Name < b.Instance.Name
}

func (m *Model) serviceCells() [][]string {
	cells := make([][]string, len(m.serviceRows))
	for i, row := range m.serviceRows {
		cells[i] = presentation.GetServiceRowDisplayStrings(&m.cfg.UserConfig.Gui, row, m.upstreams)
	}

	return cells
}

func (m *Model) serviceFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    presentation.ServiceImageColumn(&m.cfg.UserConfig.Gui),
		MinWidth: presentation.MinImageAliasWidth,
	}}
}

func (m *Model) selectedServiceRow() *commands.ServiceRow {
	item := m.panelNamed("services").item()
	if item < 0 || item >= len(m.serviceRows) {
		return nil
	}

	return m.serviceRows[item]
}

// serviceKey is pkg/gui's GetItemContextCacheKey for the selected row.
func (m *Model) serviceKey() string {
	row := m.selectedServiceRow()
	if row == nil {
		return ""
	}

	// Each refresh builds new ComposeService values, so the instance count
	// is part of the key: a replica coming or going must restart the tab.
	key := "services-" + row.Key() + "-" + row.Service.Status() + "-" + strconv.Itoa(len(row.Service.Instances))

	if row.Instance != nil {
		key += "-" + row.Instance.Instance.Status
	}

	// The config of every instance the row stands for, so the tabs follow a change to one.
	for _, instance := range row.Instances() {
		key += "-" + instance.ConfigFingerprint()
	}

	return key
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
	row := m.selectedServiceRow()
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
