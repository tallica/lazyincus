package gui

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// selectedServiceRow is the services panel's selection. No selection is a
// silent no-op, matching the other panel handlers.
func (gui *Gui) selectedServiceRow() (*commands.ServiceRow, bool) {
	row, err := gui.Panels.Services.GetSelectedItem()
	if err != nil {
		return nil, false
	}

	return row, true
}

// selectedService is the service the selection belongs to - a replica's row
// answers with its own, for the verbs that only a service has.
func (gui *Gui) selectedService() (*commands.ComposeService, bool) {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil, false
	}

	return row.Service, true
}

// composeRowDescription is what a key whose two scopes aren't the same verb
// does from the row selected right now - `d` downs a service but deletes a
// replica. The menu rebuilds its bindings each time it opens, so it can say
// which; the panels don't exist yet at the first call, which is startup
// binding the keys rather than anyone reading them.
func (gui *Gui) composeRowDescription(service, replica string) string {
	if gui.Panels.Services == nil {
		return service
	}

	if row, ok := gui.selectedServiceRow(); ok && row.Instance != nil {
		return replica
	}

	return service
}

// serviceScopedDescription is a verb only a service has, described from a
// replica's row: it still runs against the service, and saying so is what
// keeps "bring up" off a row where bringing one replica up isn't a thing.
func (gui *Gui) serviceScopedDescription(description string) string {
	return gui.composeRowDescription(
		description, fmt.Sprintf(gui.Tr.ComposeServiceScoped, description))
}

// onServiceRow splits a key between the compose verb for a whole service
// and the instances panel's own action for one replica. Only a replica's
// row takes the instance side: a service with a single instance is still
// the compose scope, where `u` and `d` create and destroy what the compose
// file declares.
func (gui *Gui) onServiceRow(
	instanceAction func(*commands.Instance) error,
	serviceAction func(*commands.ComposeService) error,
) error {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil
	}

	if row.Instance != nil {
		return instanceAction(row.Instance)
	}

	return serviceAction(row.Service)
}

// composeRun is every compose verb: the arguments, then the service to
// narrow them to. An empty service means the whole project, which is what
// the verb does with no SERVICE argument.
func (gui *Gui) composeRun(service string, args ...string) error {
	if service != "" {
		args = append(args, service)
	}

	if err := gui.runSubprocess(gui.OSCommand.NewCmd("incus-compose", args...)); err != nil {
		return err
	}

	gui.refreshInBackground(gui.fetchInstances, gui.fetchServices)

	return nil
}

// composeTarget names what a confirmation prompt is about - the service, or
// the whole project when no service narrows it.
func (gui *Gui) composeTarget(service string) string {
	if service != "" {
		return fmt.Sprintf(gui.Tr.ComposeTargetService, service)
	}

	return fmt.Sprintf(gui.Tr.ComposeTargetProject, gui.State.LocalComposeProject)
}

// composeConfirm wraps a verb in a confirmation naming its target, for the
// ones that interrupt something running or destroy it.
func (gui *Gui) composeConfirm(prompt, service string, args ...string) error {
	message := fmt.Sprintf(prompt, gui.composeTarget(service))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.composeRun(service, args...)
	}, nil)
}

func (gui *Gui) handleComposeUp(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	// --detach: without it, `up` stays attached tailing every service's logs,
	// which would leave the keypress looking hung until the user Ctrl-C's it.
	return gui.composeRun(service.Name, "up", "--detach")
}

// handleComposeUpPullRecreate is `U`: `up --pull always --recreate`,
// replacing an instance already running an older image.
func (gui *Gui) handleComposeUpPullRecreate(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeConfirm(gui.Tr.ConfirmComposeUpPullRecreate, service.Name,
		"up", "--pull", "always", "--recreate", "--detach")
}

// handleComposeStart runs `incus-compose start` - already-created instances
// only, unlike `u`, which also creates whatever's missing. incus-compose
// leaves a service's dependencies stopped unless asked, so when any are,
// a menu asks.
func (gui *Gui) handleComposeStart(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceStart, func(service *commands.ComposeService) error {
		stopped := service.StoppedDependencies(gui.composeServices())
		if len(stopped) == 0 {
			return gui.composeRun(service.Name, "start")
		}

		return gui.Menu(CreateMenuOptions{
			Title: gui.Tr.ComposeStartMenuTitle,
			Items: []*types.MenuItem{
				{
					Label: fmt.Sprintf(gui.Tr.ComposeStartWithDeps, service.Name, strings.Join(stopped, ", ")),
					OnPress: func() error {
						return gui.composeRun(service.Name, "start", "--with-deps")
					},
				},
				{
					Label: fmt.Sprintf(gui.Tr.ComposeStartOnly, service.Name),
					OnPress: func() error {
						return gui.composeRun(service.Name, "start")
					},
				},
			},
		})
	})
}

// composeServices is every service in the panel, once each.
func (gui *Gui) composeServices() []*commands.ComposeService {
	rows := gui.Panels.Services.List.GetAllItems()
	services := make([]*commands.ComposeService, 0, len(rows))

	for _, row := range rows {
		if row.Instance == nil {
			services = append(services, row.Service)
		}
	}

	return services
}

func (gui *Gui) handleComposeStop(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceStop, func(service *commands.ComposeService) error {
		return gui.composeConfirm(gui.Tr.ConfirmComposeStop, service.Name, "stop")
	})
}

func (gui *Gui) handleComposeRestart(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceRestart, func(service *commands.ComposeService) error {
		return gui.composeRun(service.Name, "restart")
	})
}

// handleComposeDown is `d`: the service's `down` submenu, or - on a
// replica's row - deleting that instance, which incus-compose creates again
// on the next `up`, the compose file still asking for it.
func (gui *Gui) handleComposeDown(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceDelete, func(service *commands.ComposeService) error {
		return gui.composeDownMenu(service.Name)
	})
}

func (gui *Gui) composeDownMenu(service string) error {
	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ComposeDownMenuTitle,
		Items: []*types.MenuItem{
			{
				Label: gui.Tr.ComposeDownOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDown, service, "down")
				},
			},
			{
				Label: gui.Tr.ComposeDownWithVolumesOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDownWithVolumes, service, "down", "--volumes")
				},
			},
		},
	})
}

func (gui *Gui) handleComposeKill(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceForceStop, func(service *commands.ComposeService) error {
		return gui.composeConfirm(gui.Tr.ConfirmComposeKill, service.Name, "kill")
	})
}

// handleComposePause is `p`, the toggle the instances panel's own `p` is -
// over the whole service, or over the one replica whose row is selected.
func (gui *Gui) handleComposePause(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instancePauseFreeze, func(service *commands.ComposeService) error {
		if len(service.Instances) == 0 {
			return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
		}

		return gui.composeRun(service.Name, composePauseVerb(service.Status()))
	})
}

// composePauseVerb is which half of the toggle to run, over one service's
// rolled-up status or every service's at once: frozen throughout thaws,
// anything else freezes. Services with nothing running don't vote.
func composePauseVerb(statuses ...string) string {
	frozen := false

	for _, status := range statuses {
		if status == commands.ServiceNone {
			continue
		}

		if !strings.EqualFold(status, "Frozen") {
			return "pause"
		}

		frozen = true
	}

	if frozen {
		return "unpause"
	}

	return "pause"
}

func (gui *Gui) handleComposeBuild(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "build")
}

func (gui *Gui) handleComposePull(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "pull")
}

// handleComposeProjectMenu is `C`: the same verbs the service keys run, with
// the SERVICE argument left off so they act on the whole stack. It takes no
// selection - a project whose services have never been deployed is brought
// up from here.
func (gui *Gui) handleComposeProjectMenu(g *gocui.Gui, v *gocui.View) error {
	// One pause row rather than two, the way `p` is one key: the verb is the
	// stack's own status, every service voting.
	services := gui.composeServices()
	statuses := make([]string, 0, len(services))

	for _, service := range services {
		statuses = append(statuses, service.Status())
	}

	pauseVerb := composePauseVerb(statuses...)

	pauseLabel := gui.Tr.ComposePause
	if pauseVerb == "unpause" {
		pauseLabel = gui.Tr.ComposeUnpause
	}

	item := func(label, confirm string, args ...string) *types.MenuItem {
		return &types.MenuItem{Label: label, OnPress: gui.composeMenuAction("", confirm, args)}
	}

	items := []*types.MenuItem{
		item(gui.Tr.ComposeUp, "", "up", "--detach"),
		// Down is the service key's own submenu, `--volumes` and all, with the
		// project as its target.
		{Label: gui.Tr.ComposeDown, OnPress: func() error { return gui.composeDownMenu("") }},
		item(gui.Tr.ComposeUpPullRecreate, gui.Tr.ConfirmComposeUpPullRecreate,
			"up", "--pull", "always", "--recreate", "--detach"),
		item(gui.Tr.Start, "", "start"),
		item(gui.Tr.Stop, gui.Tr.ConfirmComposeStop, "stop"),
		item(gui.Tr.Restart, "", "restart"),
		item(pauseLabel, "", pauseVerb),
		item(gui.Tr.ComposeKill, gui.Tr.ConfirmComposeKill, "kill"),
		item(gui.Tr.ComposeBuild, "", "build"),
		item(gui.Tr.ComposePull, "", "pull"),
		item(gui.Tr.ComposeLogs, "", "logs", "--follow"),
	}

	return gui.Menu(CreateMenuOptions{
		Title: fmt.Sprintf(gui.Tr.ComposeProjectMenuTitle, gui.State.LocalComposeProject),
		Items: items,
	})
}

func (gui *Gui) composeMenuAction(service, confirm string, args []string) func() error {
	return func() error {
		if confirm != "" {
			return gui.composeConfirm(confirm, service, args...)
		}

		return gui.composeRun(service, args...)
	}
}

// withServiceInstance runs a per-instance action against the instance the
// selected row stands for, and acts straight away when there is one. A
// service's own row with replicas under it still asks which.
func (gui *Gui) withServiceInstance(title string, action func(*commands.Instance) error) error {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil
	}

	if len(row.Service.Instances) == 0 {
		return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
	}

	if instance, ok := row.SelectedInstance(); ok {
		return action(instance)
	}

	items := make([]*types.MenuItem, 0, len(row.Service.Instances))

	for _, instance := range row.Service.SortedInstances() {
		items = append(items, &types.MenuItem{
			Label:   instance.Name,
			OnPress: func() error { return action(instance) },
		})
	}

	return gui.Menu(CreateMenuOptions{Title: title, Items: items})
}

func (gui *Gui) handleServiceExecShell(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.ExecShell, gui.instanceExecShell)
}

func (gui *Gui) handleServiceCopyIPv4(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.CopyIPv4, gui.instanceCopyIPv4)
}

func (gui *Gui) handleServiceSnapshotCreate(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.NewSnapshot, gui.snapshotCreatePrompt)
}

func (gui *Gui) handleServiceViewLogs(g *gocui.Gui, v *gocui.View) error {
	if err := gui.Panels.Services.SetMainTab("logs"); err != nil {
		return err
	}

	return gui.switchFocus(gui.Views.Main)
}
