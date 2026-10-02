package gui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
			return presentation.GetServiceRowDisplayStrings(&gui.Config.UserConfig.Gui, row)
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
	// The same column the instance blocks below use: the tab is one run of
	// labels, the service's and then its instances'.
	padding := identityPadding

	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := line("Service", service.Name)
	output += line("Image", service.ResolvedImage())
	output += line("Replicas", presentation.ServiceReplicas(service))
	output += line("Health", service.Health())

	if project := gui.composeProject.Load(); project != nil {
		output += line("Healthcheck", gui.composeHealthcheckStr(project))
	}

	output += line("Command", service.Command)
	output += line("Restart", service.Restart)
	output += line("Ports", strings.Join(service.Ports, ", "))
	output += line("Volumes", strings.Join(service.Volumes, ", "))
	output += line("Devices", strings.Join(service.Devices, ", "))
	output += line("Depends on", strings.Join(service.DependsOn, ", "))

	if len(service.Instances) == 0 {
		return output + "\n" + gui.Tr.ServiceNotRunning
	}

	// Each instance's own Info tab under that - the selected replica's
	// alone, or every one of them from the service's own row - each under a
	// heading of its own, and minus the lines the service or that heading
	// has just said: project and image are the same for every replica, the
	// name is the heading's, and a lone instance's health is what the
	// service's rolls up to.
	omit := []string{"Project", "Image", "Name"}
	if len(service.Instances) == 1 {
		omit = append(omit, "Health")
	}

	for _, instance := range row.Instances() {
		output += "\n" + gui.instanceHeading(service, instance) + "\n"
		output += "\n" + gui.instanceInfoStr(instance.Latest(), omit...)
	}

	return output
}

// instanceHeading rules off one instance's block and says which of the
// service's instances it is - a name among a dozen identical labels not
// being enough to place you. The number is over the service's instances
// rather than the ones on screen, so a replica's own row still reads "2 of
// 4". A lone instance has no count worth printing, and one carrying the
// service's own name has nothing left to say: the Service line said it.
func (gui *Gui) instanceHeading(service *commands.ComposeService, instance *commands.Instance) string {
	instances := service.SortedInstances()

	if len(instances) < 2 {
		if instance.Name == service.Name {
			return gui.sectionHeading(gui.Tr.InstanceTitle)
		}

		return gui.sectionHeading(fmt.Sprintf(gui.Tr.ServiceInstanceHeading, instance.Name))
	}

	index := slices.IndexFunc(instances, func(other *commands.Instance) bool {
		return other.Name == instance.Name
	})

	return gui.sectionHeading(fmt.Sprintf(
		gui.Tr.ServiceReplicaHeading, index+1, len(instances), instance.Name))
}

func (gui *Gui) composeHealthcheckStr(project *commands.ComposeProject) string {
	if !project.HealthcheckEnabled() {
		return gui.Tr.No
	}

	if scope := project.HealthcheckScope(); scope != "" {
		return fmt.Sprintf("%s (scope: %s)", gui.Tr.Yes, scope)
	}

	return gui.Tr.Yes
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
				return gui.serviceNoSingleInstanceStr(row.Service)
			})
		}

		return render(instance)
	}
}

// serviceNoSingleInstanceStr says which of the two reasons there's no
// instance to show: nothing running, or a row per replica to pick from.
func (gui *Gui) serviceNoSingleInstanceStr(service *commands.ComposeService) string {
	if len(service.Instances) == 0 {
		return gui.Tr.ServiceNotRunning
	}

	return gui.Tr.ServiceMultipleInstances
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

	return gui.renderLogsToMain(func() string { return gui.serviceLogsStr(row.Service, instances) })
}

func (gui *Gui) serviceLogsStr(service *commands.ComposeService, instances []*commands.Instance) string {
	sections := make([]string, 0, len(instances))

	for _, instance := range instances {
		sections = append(sections, gui.instanceHeading(service, instance)+"\n\n"+strings.TrimRight(gui.instanceLogStr(instance), "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func (gui *Gui) renderServiceConfig(row *commands.ServiceRow) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.serviceConfigStr(row) })
}

// serviceConfigStr is both halves of a service's configuration: what the
// compose file declares, then what the daemon made of it.
func (gui *Gui) serviceConfigStr(row *commands.ServiceRow) string {
	service := row.Service
	output := gui.sectionHeading(gui.Tr.ComposeTitle) + "\n\n" + gui.composeServiceConfigStr(service)

	// Then what the daemon made of it: the same dump the instances panel's
	// Config tab shows, one per instance the row stands for, under the Info
	// tab's own headings.
	for _, instance := range row.Instances() {
		output += "\n\n" + gui.instanceHeading(service, instance) +
			"\n\n" + gui.instanceConfigStr(instance)
	}

	return output
}

// composeServiceConfigStr is the selected service's slice of `incus-compose
// config`, re-rendered as YAML: the command prints the whole project, and
// the panel is already one row per service.
func (gui *Gui) composeServiceConfigStr(service *commands.ComposeService) string {
	definition, found, err := gui.IncusCommand.ComposeServiceConfig(service.Dir, service.Name)
	if err != nil {
		return fmt.Sprintf("Error running `incus-compose config`: %v", err)
	}

	if !found {
		return gui.Tr.ServiceNotInComposeFile
	}

	return utils.ColoredYamlString(definition)
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
		gui.Log.Warn(projectErr)
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

		gui.composeInstances.Store(&stackInstances{remote: stack.Remote, project: stack.Name, services: services, orphans: orphans})

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
