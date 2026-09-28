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

// composeTarget is what a compose verb acts on: a stack, by its directory,
// narrowed to one of its services unless service is empty - the verb with
// no SERVICE argument, which is the whole stack.
type composeTarget struct {
	dir     string
	project string
	service string
}

func stackTarget(stack *commands.ComposeStack) composeTarget {
	return composeTarget{dir: stack.Dir, project: stack.Name}
}

func serviceTarget(service *commands.ComposeService) composeTarget {
	return composeTarget{dir: service.Dir, project: service.Project, service: service.Name}
}

// composeRun is every compose verb: the arguments, then the service to
// narrow them to. The stack's config is read again after, in case the verb
// followed an edit to its compose file.
func (gui *Gui) composeRun(target composeTarget, args ...string) error {
	if target.service != "" {
		args = append(args, target.service)
	}

	if err := gui.runSubprocess(gui.IncusCommand.ComposeCmd(target.dir, args...)); err != nil {
		return err
	}

	gui.stacks.forget(target.dir)
	gui.refreshInBackground(gui.fetchInstances, gui.fetchStacks, gui.fetchServices)

	return nil
}

// composeTargetName names what a confirmation prompt is about - the service,
// or the whole project when no service narrows it.
func (gui *Gui) composeTargetName(target composeTarget) string {
	if target.service != "" {
		return fmt.Sprintf(gui.Tr.ComposeTargetService, target.service)
	}

	return fmt.Sprintf(gui.Tr.ComposeTargetProject, target.project)
}

// composeConfirm wraps a verb in a confirmation naming its target, for the
// ones that interrupt something running or destroy it.
func (gui *Gui) composeConfirm(prompt string, target composeTarget, args ...string) error {
	message := fmt.Sprintf(prompt, gui.composeTargetName(target))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.composeRun(target, args...)
	}, nil)
}

// composeUp is `up --detach`: without it, `up` stays attached tailing every
// service's logs, which would leave the keypress looking hung until the
// user Ctrl-C's it.
func (gui *Gui) composeUp(target composeTarget) error {
	return gui.composeRun(target, "up", "--detach")
}

// composeUpPullRecreate is `U`: `up --pull always --recreate`, replacing an
// instance already running an older image.
func (gui *Gui) composeUpPullRecreate(target composeTarget) error {
	return gui.composeConfirm(gui.Tr.ConfirmComposeUpPullRecreate, target,
		"up", "--pull", "always", "--recreate", "--detach")
}

func (gui *Gui) handleComposeUp(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeUp(serviceTarget(service))
}

func (gui *Gui) handleComposeUpPullRecreate(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeUpPullRecreate(serviceTarget(service))
}

// handleComposeStart runs `incus-compose start` - already-created instances
// only, unlike `u`, which also creates whatever's missing. incus-compose
// leaves a service's dependencies stopped unless asked, so when any are,
// a menu asks.
func (gui *Gui) handleComposeStart(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceStart, func(service *commands.ComposeService) error {
		target := serviceTarget(service)

		stopped := service.StoppedDependencies(gui.composeServices())
		if len(stopped) == 0 {
			return gui.composeRun(target, "start")
		}

		return gui.Menu(CreateMenuOptions{
			Title: gui.Tr.ComposeStartMenuTitle,
			Items: []*types.MenuItem{
				{
					Label: fmt.Sprintf(gui.Tr.ComposeStartWithDeps, service.Name, strings.Join(stopped, ", ")),
					OnPress: func() error {
						return gui.composeRun(target, "start", "--with-deps")
					},
				},
				{
					Label: fmt.Sprintf(gui.Tr.ComposeStartOnly, service.Name),
					OnPress: func() error {
						return gui.composeRun(target, "start")
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
		return gui.composeConfirm(gui.Tr.ConfirmComposeStop, serviceTarget(service), "stop")
	})
}

func (gui *Gui) handleComposeRestart(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceRestart, func(service *commands.ComposeService) error {
		return gui.composeRun(serviceTarget(service), "restart")
	})
}

// handleComposeDown is `d`: the service's `down` submenu, or - on a
// replica's row - deleting that instance, which incus-compose creates again
// on the next `up`, the compose file still asking for it.
func (gui *Gui) handleComposeDown(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceDelete, func(service *commands.ComposeService) error {
		return gui.composeDownMenu(serviceTarget(service))
	})
}

func (gui *Gui) composeDownMenu(target composeTarget) error {
	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ComposeDownMenuTitle,
		Items: []*types.MenuItem{
			{
				Label: gui.Tr.ComposeDownOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDown, target, "down")
				},
			},
			{
				Label: gui.Tr.ComposeDownWithVolumesOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDownWithVolumes, target, "down", "--volumes")
				},
			},
		},
	})
}

func (gui *Gui) handleComposeKill(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceForceStop, func(service *commands.ComposeService) error {
		return gui.composeConfirm(gui.Tr.ConfirmComposeKill, serviceTarget(service), "kill")
	})
}

// handleComposePause is `p`, the toggle the instances panel's own `p` is -
// over the whole service, or over the one replica whose row is selected.
func (gui *Gui) handleComposePause(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instancePauseResume, func(service *commands.ComposeService) error {
		if len(service.Instances) == 0 {
			return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
		}

		return gui.composeRun(serviceTarget(service), composePauseVerb(service.Status()))
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

	return gui.composeRun(serviceTarget(service), "build")
}

func (gui *Gui) handleComposePull(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(serviceTarget(service), "pull")
}

// handleComposeProjectMenu is `C`: the same verbs the service keys run, with
// the SERVICE argument left off so they act on the whole stack. It takes no
// selection - a project whose services have never been deployed is brought
// up from here.
func (gui *Gui) handleComposeProjectMenu(g *gocui.Gui, v *gocui.View) error {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return nil
	}

	services := gui.composeServices()
	statuses := make([]string, 0, len(services))

	for _, service := range services {
		statuses = append(statuses, service.Status())
	}

	return gui.composeStackMenu(stackTarget(stack), statuses)
}

// composeStackMenu is `C`'s menu for the stack target names, statuses being
// its services' rolled up.
func (gui *Gui) composeStackMenu(target composeTarget, statuses []string) error {
	// One pause row rather than two, the way `p` is one key: the verb is the
	// stack's own status, every service voting.
	pauseVerb := composePauseVerb(statuses...)

	pauseLabel := gui.Tr.ComposePause
	if pauseVerb == "unpause" {
		pauseLabel = gui.Tr.ComposeUnpause
	}

	item := func(label, confirm string, args ...string) *types.MenuItem {
		return &types.MenuItem{Label: label, OnPress: gui.composeMenuAction(target, confirm, args)}
	}

	items := []*types.MenuItem{
		item(gui.Tr.ComposeUp, "", "up", "--detach"),
		// Down is the service key's own submenu, `--volumes` and all, with the
		// project as its target.
		{Label: gui.Tr.ComposeDown, OnPress: func() error { return gui.composeDownMenu(target) }},
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
		Title: fmt.Sprintf(gui.Tr.ComposeProjectMenuTitle, target.project),
		Items: items,
	})
}

func (gui *Gui) composeMenuAction(target composeTarget, confirm string, args []string) func() error {
	return func() error {
		if confirm != "" {
			return gui.composeConfirm(confirm, target, args...)
		}

		return gui.composeRun(target, args...)
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

func (gui *Gui) handleServiceEdit(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.EditInEditor, gui.instanceEdit)
}

func (gui *Gui) handleServiceExecShell(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.ExecShell, gui.instanceExecShell)
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
