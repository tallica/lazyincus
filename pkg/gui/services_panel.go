package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// getServicesPanel lists a service per row, and - when it has replicas -
// one row per replica under it.
func (gui *Gui) getServicesPanel() *panels.SideListPanel[*commands.ServiceRow] {
	return &panels.SideListPanel[*commands.ServiceRow]{
		ContextState: &panels.ContextState[*commands.ServiceRow]{
			GetMainTabs: func() []panels.MainTab[*commands.ServiceRow] {
				return []panels.MainTab[*commands.ServiceRow]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderServiceInfo,
					},
					{
						Key:    "logs",
						Title:  gui.Tr.LogsTitle,
						Render: gui.renderServiceLogs,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderServiceConfig,
					},
					{
						Key:    "env",
						Title:  gui.Tr.EnvTitle,
						Render: gui.serviceInstanceTab(gui.renderInstanceEnv),
					},
					{
						Key:    "top",
						Title:  gui.Tr.TopTitle,
						Render: gui.serviceInstanceTab(gui.renderInstanceTopToMain),
					},
				}
			},
			GetItemContextCacheKey: func(row *commands.ServiceRow) string {
				// Each refresh builds new ComposeService values, so the
				// instance count is part of the key: without it a replica
				// coming or going leaves the main panel rendering the
				// service object the tab opened with. A replica row adds its
				// own status, the way the instances panel does, so a restart
				// re-reads the log.
				key := "services-" + row.Key() + "-" + row.Service.Status() +
					"-" + strconv.Itoa(len(row.Service.Instances))

				if row.Instance != nil {
					key += "-" + row.Instance.Instance.Status
				}

				// The config of every instance the row stands for - a
				// service's own row too, whose Config tab shows each - so
				// the tabs follow a change to one.
				for _, instance := range row.Instances() {
					key += "-" + instance.ConfigFingerprint()
				}

				return key
			},
		},
		ListPanel: panels.ListPanel[*commands.ServiceRow]{
			List: panels.NewFilteredList[*commands.ServiceRow](),
			View: gui.Views.Services,
		},
		NoItemsMessage: gui.Tr.NoStackSelected,
		EmptyNote:      func() string { return gui.State.ServicesNote },
		Gui:            gui.intoInterface(),
		// The snapshots panel shows the selected row's instances while this
		// panel has focus, the way it follows the instances panel. A
		// service's own row means every replica of it, so the panel holds
		// all of their snapshots, each row naming the replica it came from.
		OnSelect: func(row *commands.ServiceRow) error {
			label := row.Service.Name
			if row.Instance != nil {
				label = row.Instance.Name
			}

			return gui.refreshSnapshotsFor(label, row.Service.Remote, row.Instances()...)
		},
		Hide: gui.composeUnavailable,
		// Compose file order is arbitrary (a JSON object), so name is the
		// only stable order there is; a service's replicas follow it, named
		// in order themselves.
		Sort: func(a, b *commands.ServiceRow) bool {
			if a.Service.Name != b.Service.Name {
				return a.Service.Name < b.Service.Name
			}

			if (a.Instance == nil) != (b.Instance == nil) {
				return a.Instance == nil
			}

			return a.Instance != nil && a.Instance.Name < b.Instance.Name
		},
		// Rows are rebuilt on every refresh, so the cursor follows the one
		// it was on by name rather than by pointer.
		SameItem: func(a, b *commands.ServiceRow) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(row *commands.ServiceRow) []string {
			return presentation.GetServiceRowDisplayStrings(&gui.Config.UserConfig.Gui, row, gui.upstreams())
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index:    presentation.ServiceImageColumn(&gui.Config.UserConfig.Gui),
				MinWidth: presentation.MinImageAliasWidth,
			}}
		},
	}
}

// renderServiceInfo ticks like the instances panel's Stats tab, the two
// being one tab here: the identity half is static, the counters underneath
// it aren't.
func (gui *Gui) renderServiceInfo(row *commands.ServiceRow) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.serviceInfoStr(row))
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) serviceInfoStr(row *commands.ServiceRow) string {
	service := row.Service

	remote := service.Remote
	if remote == "" {
		remote = gui.IncusCommand.RemoteName()
	}

	loc := location{Remote: remote, Project: service.Project, Stack: gui.stackLabel(remote, service.Project)}
	verdict := gui.upstreamVerdict(commands.MostTelling(gui.upstreams(), presentation.BaseImages(row.Instances())...))

	return presentation.ServiceInfo(row, loc, verdict, gui.composeProject.Load(), gui.instanceInfoStr,
		int(gui.mainViewWidth.Load()), gui.Tr)
}

// serviceInstanceTab is a tab that is really the instance's: the row's
// replica answers for it, as does a lone service's only instance - the same
// split the per-instance keys make through withServiceInstance.
func (gui *Gui) serviceInstanceTab(
	render func(*commands.Instance) tasks.TaskFunc,
) func(*commands.ServiceRow) tasks.TaskFunc {
	return func(row *commands.ServiceRow) tasks.TaskFunc {
		instance, ok := row.SelectedInstance()
		if !ok {
			return gui.NewSimpleRenderStringTask(func() string {
				return presentation.ServiceNoSingleInstance(row.Service, gui.Tr)
			})
		}

		return render(instance)
	}
}

// renderServiceLogs delegates to the instance logs renderer, which owns the
// drain-on-read console buffer. A replicated service's own row has no
// single stream to show, so it stacks every replica's under a heading of
// its own rather than interleaving them - the buffers carry no timestamps
// to merge on. `M`'s `logs --follow` is the merged view.
func (gui *Gui) renderServiceLogs(row *commands.ServiceRow) tasks.TaskFunc {
	if instance, ok := row.SelectedInstance(); ok {
		return gui.renderInstanceLogsToMain(instance)
	}

	instances := row.Instances()
	if len(instances) == 0 {
		return gui.NewSimpleRenderStringTask(func() string { return gui.Tr.ServiceNotRunning })
	}

	return gui.renderLogsToMain(func() string {
		return presentation.ServiceLogs(row.Service, instances, int(gui.mainViewWidth.Load()), gui.instanceLogStr, gui.Tr)
	})
}

func (gui *Gui) renderServiceConfig(row *commands.ServiceRow) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.serviceConfigStr(row) })
}

func (gui *Gui) serviceConfigStr(row *commands.ServiceRow) string {
	definition, found, err := gui.IncusCommand.ComposeServiceConfig(row.Service.Dir, row.Service.Name)

	return presentation.ServiceConfig(row, presentation.ComposeServiceConfig(definition, found, err, gui.Tr),
		int(gui.mainViewWidth.Load()), gui.Tr)
}

// fetchServices lists the services of the stack the Stacks panel has
// selected. The ticket comes before the stack is read: followStack swaps the
// stack before it invalidates, so a fetch it doesn't turn away has the new
// one.
func (gui *Gui) fetchServices() (func() error, error) {
	if gui.composeUnavailable() {
		return func() error { return nil }, nil
	}

	ticket := gui.refreshes.services.issue()

	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return func() error {
			if !gui.refreshes.services.admit(ticket) {
				return nil
			}

			gui.Panels.Services.SetItems(nil)

			return gui.Panels.Services.RerenderList()
		}, nil
	}

	command, err := gui.commandFor(stack.Remote)
	if err == nil && !gui.onSessionRemote(stack.Remote) {
		err = gui.remotes.remoteStatusErr(stack.Remote)
	}

	var (
		services []*commands.ComposeService
		orphans  []*commands.Instance
	)

	if err == nil {
		services, orphans, err = command.GetComposeServices(stack)
	}

	if err != nil {
		if gui.onSessionRemote(stack.Remote) {
			return nil, err
		}

		note := fmt.Sprintf(gui.Tr.CannotReachRemote, stack.Remote)
		if errors.Is(err, errConnecting) {
			err = fmt.Errorf(gui.Tr.ConnectingTo, stack.Remote)
			note = err.Error()
		}

		// Another remote's silence is its stack's row's to report, not a
		// popup on every poll.
		return gui.showServicesUnreachable(ticket, err, note), nil
	}

	// The project's own config backs the Info tabs' healthcheck line;
	// fetched here so rendering stays free of API calls.
	project, projectErr := command.GetComposeProject(stack.Name)
	if projectErr != nil {
		gui.Log.Warn().Err(projectErr).Send()
	}

	return func() error {
		if !gui.refreshes.services.admit(ticket) {
			return nil
		}

		// A nil project is an answer - no longer compose-managed - where an
		// error isn't one.
		if projectErr == nil {
			gui.composeProject.Store(project)
		}

		gui.composeInstances.Store(&stackInstances{Remote: stack.Remote, Project: stack.Name, Services: services, Orphans: orphans})

		gui.Panels.Services.NoItemsMessage = gui.Tr.NoServices
		gui.State.ServicesNote = ""
		gui.Panels.Services.SetItems(commands.ServiceRows(services))

		if err := gui.Panels.Services.RerenderList(); err != nil {
			return err
		}

		return gui.renderSnapshots()
	}, nil
}

// showServicesUnreachable empties the services panel for a stack whose
// remote didn't answer, a note saying so in the list and why for when the
// panel has focus.
func (gui *Gui) showServicesUnreachable(ticket uint64, err error, note string) func() error {
	return func() error {
		if !gui.refreshes.services.admit(ticket) {
			return nil
		}

		gui.composeInstances.Store(nil)
		gui.State.ServicesNote = note
		gui.Panels.Services.NoItemsMessage = err.Error()
		gui.Panels.Services.SetItems(nil)

		return gui.Panels.Services.RerenderList()
	}
}
