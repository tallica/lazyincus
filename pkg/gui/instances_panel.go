package gui

import (
	"fmt"

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
						Key:    "logs",
						Title:  gui.Tr.LogsTitle,
						Render: gui.renderInstanceLogsToMain,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderInstanceConfig,
					},
				}
			},
			GetItemContextCacheKey: func(instance *commands.Instance) string {
				// Including the instance status in the cache key so that if the
				// instance restarts we re-read the logs.
				return "instances-" + instance.Name + "-" + instance.Instance.Status
			},
		},
		ListPanel: panels.ListPanel[*commands.Instance]{
			List: panels.NewFilteredList[*commands.Instance](),
			View: gui.Views.Instances,
		},
		NoItemsMessage: gui.Tr.NoInstances,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Instance, b *commands.Instance) bool {
			return sortInstances(a, b)
		},
		Filter: func(instance *commands.Instance) bool {
			if !gui.State.ShowStoppedInstances && instance.Instance.Status == "Stopped" {
				return false
			}
			return true
		},
		GetTableCells: func(instance *commands.Instance) []string {
			return presentation.GetInstanceDisplayStrings(&gui.Config.UserConfig.Gui, instance)
		},
	}
}

var instanceStates = map[string]int{
	"Running": 1,
	"Frozen":  2,
	"Stopped": 3,
	"Error":   4,
}

func sortInstances(a *commands.Instance, b *commands.Instance) bool {
	stateLeft := instanceStates[a.Instance.Status]
	stateRight := instanceStates[b.Instance.Status]
	if stateLeft == stateRight {
		return a.Name < b.Name
	}

	return stateLeft < stateRight
}

func (gui *Gui) renderInstanceConfig(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceConfigStr(instance) })
}

func (gui *Gui) instanceConfigStr(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok {
		return gui.Tr.WaitingForInstanceInfo
	}

	padding := 10
	output := ""
	output += utils.WithPadding("Name: ", padding) + full.Name + "\n"
	output += utils.WithPadding("Type: ", padding) + full.Type + "\n"
	output += utils.WithPadding("Status: ", padding) + full.Status + "\n"
	output += utils.WithPadding("Created: ", padding) + full.CreatedAt.String() + "\n"
	output += utils.WithPadding("Profiles: ", padding) + fmt.Sprint(full.Profiles) + "\n"

	data, err := utils.MarshalIntoYaml(full)
	if err != nil {
		return fmt.Sprintf("Error marshalling instance details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) refreshInstances() error {
	if gui.Views.Instances == nil {
		return nil
	}

	instances, err := gui.IncusCommand.GetInstances(gui.Panels.Instances.List.GetAllItems())
	if err != nil {
		return err
	}

	gui.Panels.Instances.SetItems(instances)

	return gui.Panels.Instances.RerenderList()
}

func (gui *Gui) handleHideStoppedInstances(g *gocui.Gui, v *gocui.View) error {
	gui.State.ShowStoppedInstances = !gui.State.ShowStoppedInstances

	return gui.Panels.Instances.RerenderList()
}

func (gui *Gui) handleInstanceStart(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.WithWaitingStatus(gui.Tr.StartingStatus, func() error {
		if err := inst.Start(); err != nil {
			return gui.createErrorPanel(err.Error())
		}
		return gui.refreshInstances()
	})
}

func (gui *Gui) handleInstanceStop(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.createConfirmationPanel(gui.Tr.Confirm, gui.Tr.StopInstance, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.StoppingStatus, func() error {
			if err := inst.Stop(); err != nil {
				return gui.createErrorPanel(err.Error())
			}
			return gui.refreshInstances()
		})
	}, nil)
}

func (gui *Gui) handleInstanceRestart(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.WithWaitingStatus(gui.Tr.RestartingStatus, func() error {
		if err := inst.Restart(); err != nil {
			return gui.createErrorPanel(err.Error())
		}
		return gui.refreshInstances()
	})
}

func (gui *Gui) handleInstancePauseFreeze(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.WithWaitingStatus(gui.Tr.PausingStatus, func() (err error) {
		if inst.Instance.Status == "Frozen" {
			err = inst.Unfreeze()
		} else {
			err = inst.Freeze()
		}

		if err != nil {
			return gui.createErrorPanel(err.Error())
		}

		return gui.refreshInstances()
	})
}

func (gui *Gui) handleInstanceDelete(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.createConfirmationPanel(gui.Tr.Confirm, gui.Tr.DeleteInstance, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := inst.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}
			return gui.refreshInstances()
		})
	}, nil)
}

func (gui *Gui) handleInstanceViewLogs(g *gocui.Gui, v *gocui.View) error {
	gui.Panels.Instances.SetMainTabIndex(0)
	return gui.handleEnterMain(g, v)
}

func (gui *Gui) handleInstancesExecShell(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.instanceExecShell(inst)
}

// instanceExecShell shells out to the incus CLI to exec an interactive shell
// into the instance. We use the CLI here (mirroring how lazydocker shells out
// to `docker exec`/`docker attach`) rather than driving the client library's
// websocket-based ExecInstance directly, since that would require plumbing
// the TUI's suspended terminal through as the exec session's stdio.
func (gui *Gui) instanceExecShell(instance *commands.Instance) error {
	if !instance.IsRunning() {
		return gui.createErrorPanel(gui.Tr.CannotAttachStoppedInstanceError)
	}

	cmd := gui.OSCommand.NewCmd("incus", "exec", instance.Name, "--", "sh", "-c",
		"exec $(command -v bash || command -v ash || command -v sh)")

	return gui.runSubprocessWithMessage(cmd, gui.Tr.DetachFromInstanceShortCut)
}
