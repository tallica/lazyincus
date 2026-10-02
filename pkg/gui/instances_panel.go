package gui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/samber/lo"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getInstancesPanel() *panels.SideListPanel[*commands.Instance] {
	return &panels.SideListPanel[*commands.Instance]{
		ContextState: &panels.ContextState[*commands.Instance]{
			GetMainTabs: func() []panels.MainTab[*commands.Instance] {
				return []panels.MainTab[*commands.Instance]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderInstanceInfoToMain,
					},
					{
						Key:    "logs",
						Title:  gui.Tr.LogsTitle,
						Render: gui.renderInstanceLogsToMain,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderInstanceConfig,
					},
					{
						Key:    "env",
						Title:  gui.Tr.EnvTitle,
						Render: gui.renderInstanceEnv,
					},
					{
						Key:    "top",
						Title:  gui.Tr.TopTitle,
						Render: gui.renderInstanceTopToMain,
					},
				}
			},
			GetItemContextCacheKey: func(instance *commands.Instance) string {
				// The status, so a restart re-reads the logs; the config, so
				// the Config and Env tabs follow a change to it.
				return "instances-" + instance.Key() + "-" + instance.Instance.Status + "-" + instance.ConfigFingerprint()
			},
		},
		ListPanel: panels.ListPanel[*commands.Instance]{
			List: panels.NewFilteredList[*commands.Instance](),
			View: gui.Views.Instances,
		},
		NoItemsMessage: gui.Tr.NoInstances,
		Gui:            gui.intoInterface(),
		// The snapshots panel shows whichever instance is selected here, so
		// it reloads whenever that changes rather than on its poll alone.
		OnSelect: func(instance *commands.Instance) error {
			return gui.refreshSnapshotsFor(instance.Name, instance)
		},
		Sort: func(a *commands.Instance, b *commands.Instance) bool {
			return sortInstances(a, b)
		},
		SameItem: func(a, b *commands.Instance) bool {
			return a.Key() == b.Key()
		},
		Filter: func(instance *commands.Instance) bool {
			// Every user, stopped or a stack's: the question was what uses
			// the resource.
			if users := gui.State.InstanceUsers; users != nil {
				return users.uses(instance)
			}

			if !gui.State.ShowStoppedInstances && isStopped(instance) {
				return false
			}

			return gui.State.ShowStackInstances || !gui.isStackInstance(instance)
		},
		GetTableCells: func(instance *commands.Instance) []string {
			return presentation.GetInstanceDisplayStrings(
				&gui.Config.UserConfig.Gui, instance, gui.State.SpansProjects.Instances)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index:    presentation.InstanceImageColumn(&gui.Config.UserConfig.Gui, gui.State.SpansProjects.Instances),
				MinWidth: presentation.MinImageAliasWidth,
			}}
		},
	}
}

// sortInstances orders by name, with stopped instances after everything
// else - a stopped instance is usually the one you're least interested in,
// but sorting by status beyond that just shuffles the list around as
// instances start and stop.
//
// In the all-projects view the project comes first, so the list reads as
// one group per project rather than names interleaved across them.
func sortInstances(a *commands.Instance, b *commands.Instance) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}

	if isStopped(a) != isStopped(b) {
		return isStopped(b)
	}

	return a.Name < b.Name
}

func isStopped(instance *commands.Instance) bool {
	return strings.EqualFold(instance.Status(), "Stopped")
}

// isStackInstance reports whether a listed stack has this instance, and so
// the services panel, which is what makes this panel the standalone one.
// A compose instance no Services row claims - of a project no stack lists,
// or of a service its compose file no longer declares - stays here.
func (gui *Gui) isStackInstance(instance *commands.Instance) bool {
	return gui.State.StackServices[instance.Project][instance.ComposeService()]
}

func (gui *Gui) renderInstanceConfig(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceConfigStr(instance) })
}

// instanceConfigStr is the dump alone: what used to head it - name, type,
// status, created, profiles - is the Info tab's identity block now.
func (gui *Gui) instanceConfigStr(instance *commands.Instance) string {
	data, err := utils.MarshalIntoYaml(instance.Instance)
	if err != nil {
		return fmt.Sprintf("Error marshalling instance details: %v", err)
	}

	return utils.ColoredYamlString(string(data))
}

func (gui *Gui) fetchInstances() (func() error, error) {
	ticket := gui.refreshes.instances.issue()

	instances, err := gui.IncusCommand.GetInstances()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.instances.admit(ticket) {
			return nil
		}

		gui.setInstancesSpan(instances)
		gui.Panels.Instances.SetItems(instances)

		if err := gui.Panels.Instances.RerenderList(); err != nil {
			return err
		}

		// The snapshots come with the instances.
		return gui.renderSnapshots()
	}, nil
}

// setInstancesSpan is computed over what the panel will actually show: with
// the stacks gone to the services panel, the instances left can sit in one
// project even when the server's don't.
func (gui *Gui) setInstancesSpan(instances []*commands.Instance) {
	standalone := lo.Reject(instances, func(instance *commands.Instance, _ int) bool {
		return !gui.State.ShowStackInstances && gui.isStackInstance(instance)
	})

	gui.State.SpansProjects.Instances = spansMultipleProjects(
		lo.Map(standalone, func(instance *commands.Instance, _ int) string { return instance.Project }))
}

func (gui *Gui) refreshInstances() error {
	return gui.refresh(nil, gui.fetchInstances)
}

// instanceUsers is what the instances panel is narrowed to: the instances
// using one image, volume or network.
type instanceUsers struct {
	label string
	uses  func(*commands.Instance) bool
	// from is the list `u` was pressed in, which esc returns to.
	from *gocui.View
}

// showUsers narrows the instances panel to what uses a resource and moves
// there; esc in the panel brings the whole list back and returns to the
// resource.
func (gui *Gui) showUsers(label string, uses func(*commands.Instance) bool) error {
	if !lo.SomeBy(gui.Panels.Instances.List.GetAllItems(), uses) {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.NothingUses, label))
	}

	gui.State.InstanceUsers = &instanceUsers{label: label, uses: uses, from: gui.g.CurrentView()}
	gui.Views.Instances.Title = gui.instancesPanelTitle()
	gui.Panels.Instances.SetSelectedLineIdx(0)

	if err := gui.Panels.Instances.RerenderList(); err != nil {
		return err
	}

	return gui.switchFocus(gui.Views.Instances)
}

func (gui *Gui) clearInstanceUsers() error {
	from := gui.State.InstanceUsers.from

	gui.State.InstanceUsers = nil
	gui.Views.Instances.Title = gui.instancesPanelTitle()

	if err := gui.Panels.Instances.RerenderList(); err != nil {
		return err
	}

	if from == nil {
		return nil
	}

	return gui.switchFocus(from)
}

func (gui *Gui) handleHideStoppedInstances(g *gocui.Gui, v *gocui.View) error {
	gui.State.ShowStoppedInstances = !gui.State.ShowStoppedInstances

	return gui.Panels.Instances.RerenderList()
}

func (gui *Gui) handleToggleStackInstances(g *gocui.Gui, v *gocui.View) error {
	gui.State.ShowStackInstances = !gui.State.ShowStackInstances
	gui.Views.Instances.Title = gui.instancesPanelTitle()
	gui.setInstancesSpan(gui.Panels.Instances.List.GetAllItems())

	return gui.Panels.Instances.RerenderList()
}

// The actions below take the instance rather than reading the selection:
// the services panel runs the same ones against the replica its selected
// row stands for. Either panel may be showing what changed, so both are
// refreshed.
func (gui *Gui) instanceStart(instance *commands.Instance) error {
	return gui.WithWaitingStatus(gui.Tr.StartingStatus, func() error {
		return gui.inTransition(instance, "Starting", instance.Start)
	})
}

func (gui *Gui) qualifiedInstance(instance *commands.Instance) string {
	return gui.qualified(instance.Name, instance.Project, gui.State.SpansProjects.Instances)
}

func (gui *Gui) instanceStop(instance *commands.Instance) error {
	message := fmt.Sprintf(gui.Tr.StopInstance, gui.qualifiedInstance(instance))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.StoppingStatus, func() error {
			return gui.inTransition(instance, "Stopping", instance.Stop)
		})
	}, nil)
}

// instanceForceStop is `incus stop --force`: the services panel's `f`, which
// kills a whole service, narrowed to one replica.
func (gui *Gui) instanceForceStop(instance *commands.Instance) error {
	message := fmt.Sprintf(gui.Tr.ForceStopInstance, gui.qualifiedInstance(instance))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.StoppingStatus, func() error {
			return gui.inTransition(instance, "Stopping", instance.ForceStop)
		})
	}, nil)
}

func (gui *Gui) instanceRestart(instance *commands.Instance) error {
	return gui.WithWaitingStatus(gui.Tr.RestartingStatus, func() error {
		return gui.inTransition(instance, "Restarting", instance.Restart)
	})
}

// inTransition shows status on the instance's row while action runs off
// the main loop, then refreshes both lists; see Instance.BeginTransition.
// The mark comes off only once the refresh has applied: before it, the
// row would fall back on the last poll, which may have caught the
// instance stopped halfway through a restart.
func (gui *Gui) inTransition(instance *commands.Instance, status string, action func() error) error {
	end := instance.BeginTransition(status)
	gui.g.Update(func(*gocui.Gui) error { return gui.rerenderInstanceLists() })

	if err := action(); err != nil {
		gui.g.Update(func(*gocui.Gui) error {
			end()
			return gui.rerenderInstanceLists()
		})

		return err
	}

	return gui.refreshEnding([]func(){end}, gui.fetchInstances, gui.fetchServices)
}

// instancePauseResume is `incus pause` or `incus resume`, whichever the
// instance's state calls for: the CLI's words for the daemon's freeze and
// unfreeze, the row reading freezing, frozen, unfreezing.
func (gui *Gui) instancePauseResume(instance *commands.Instance) error {
	if instance.Instance.Status == "Frozen" {
		return gui.WithWaitingStatus(gui.Tr.ResumingStatus, func() error {
			return gui.inTransition(instance, "Unfreezing", instance.Unfreeze)
		})
	}

	return gui.WithWaitingStatus(gui.Tr.PausingStatus, func() error {
		return gui.inTransition(instance, "Freezing", instance.Freeze)
	})
}

func (gui *Gui) instanceDelete(instance *commands.Instance) error {
	message := fmt.Sprintf(gui.Tr.DeleteInstance, gui.qualifiedInstance(instance))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			err := instance.Delete()
			if errors.Is(err, commands.ErrInstanceRunning) {
				return gui.promptToForceDeleteInstance(instance)
			}

			if err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshInstancesAndServices()
		})
	}, nil)
}

// promptToForceDeleteInstance offers to stop the instance and delete it after
// Incus refused to delete it while running. We only get here off the back of
// a delete the user already confirmed, so this second prompt is about the
// force-stop, not about the delete.
func (gui *Gui) promptToForceDeleteInstance(instance *commands.Instance) error {
	return gui.createConfirmationPanel(gui.Tr.Confirm, gui.Tr.MustForceToRemove, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.ForceRemovingStatus, func() error {
			if err := instance.ForceDelete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshInstancesAndServices()
		})
	}, nil)
}

func (gui *Gui) handleInstanceViewLogs(g *gocui.Gui, v *gocui.View) error {
	if err := gui.Panels.Instances.SetMainTab("logs"); err != nil {
		return err
	}

	return gui.handleEnterMain(g, v)
}

// instanceCmd is an `incus` command on the instance's remote and project,
// which the CLI would otherwise take from the user's own - an instance of
// a stack pinned to another remote included.
func (gui *Gui) instanceCmd(instance *commands.Instance, args ...string) *exec.Cmd {
	cmd := gui.OSCommand.NewCmd("incus", append(projectCLIArgs(instance.Project), args...)...)

	return commands.WithRemote(cmd, instance.Remote)
}

func projectCLIArgs(project string) []string {
	if project == "" {
		return nil
	}

	return []string{"--project", project}
}

// editInIncus hands the terminal to an `incus ... edit`, which opens the
// item's YAML in the user's editor and re-opens it on a validation error,
// then re-lists what it changed.
func (gui *Gui) editInIncus(project string, refresh []fetch, args ...string) error {
	return gui.runIncusEdit(gui.OSCommand.NewCmd("incus", append(projectCLIArgs(project), args...)...), refresh)
}

func (gui *Gui) runIncusEdit(cmd *exec.Cmd, refresh []fetch) error {
	if err := gui.runSubprocess(cmd); err != nil {
		return err
	}

	gui.refreshInBackground(refresh...)

	return nil
}

// instanceEdit is `incus config edit`. Incus applies what it can to a
// running instance and says so when a change waits for a restart.
func (gui *Gui) instanceEdit(instance *commands.Instance) error {
	return gui.runIncusEdit(gui.instanceCmd(instance, "config", "edit", instance.Name),
		[]fetch{gui.fetchInstances, gui.fetchServices})
}

// instanceAttachConsole shells out to `incus console`, the analog of
// lazydocker's `docker attach`: it hands the terminal to the instance's
// console rather than starting a process in it the way exec does.
func (gui *Gui) instanceAttachConsole(instance *commands.Instance) error {
	if !instance.IsRunning() {
		return gui.createErrorPanel(gui.Tr.CannotAttachStoppedInstanceError)
	}

	// `incus console` on one of these fails with the daemon's own "operation
	// not supported by device".
	if instance.IsOCI() {
		return gui.createErrorPanel(gui.Tr.CannotAttachAppContainerError)
	}

	cmd := gui.instanceCmd(instance, "console", instance.Name)

	// No detach hint from us: `incus console` prints its own on connect.
	return gui.runSubprocess(cmd)
}

// instanceExecShell shells out to the incus CLI rather than driving the
// client library's websocket ExecInstance, which would mean plumbing the
// suspended TUI's terminal through as the session's stdio.
func (gui *Gui) instanceExecShell(instance *commands.Instance) error {
	if !instance.IsRunning() {
		return gui.createErrorPanel(gui.Tr.CannotExecStoppedInstanceError)
	}

	cmd := gui.instanceCmd(instance, "exec", instance.Name, "--", "sh", "-c",
		"exec $(command -v bash || command -v ash || command -v sh)")

	return gui.runSubprocessWithMessage(cmd, gui.Tr.ExitShellToReturn)
}
