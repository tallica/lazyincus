package gui

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
		// The snapshots panel shows whichever instance is selected here, so
		// it reloads whenever that changes rather than on its poll alone.
		OnSelect: func(instance *commands.Instance) error {
			return gui.refreshSnapshotsFor(instance)
		},
		Sort: func(a *commands.Instance, b *commands.Instance) bool {
			return sortInstances(a, b)
		},
		Filter: func(instance *commands.Instance) bool {
			if !gui.State.ShowStoppedInstances && isStopped(instance) {
				return false
			}

			return !gui.isLocalComposeInstance(instance)
		},
		GetTableCells: func(instance *commands.Instance) []string {
			return presentation.GetInstanceDisplayStrings(
				&gui.Config.UserConfig.Gui, instance, gui.State.SpansProjects.Instances)
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
	return strings.EqualFold(instance.Instance.Status, "Stopped")
}

// isLocalComposeInstance reports whether the services panel already has this
// instance, which is what makes this panel the standalone one. Only the
// local stack moves: another project's compose instances have no panel of
// their own, so they stay here.
func (gui *Gui) isLocalComposeInstance(instance *commands.Instance) bool {
	if gui.noLocalComposeProject() {
		return false
	}

	if instance.Project != gui.State.LocalComposeProject {
		return false
	}

	// ComposeService() reads ExpandedConfig, which RefreshInstanceDetails
	// fills in the background - so until it has, assume an instance in the
	// stack's project is the stack's, rather than showing its rows here for
	// a second and then taking them away.
	return !instance.DetailsLoaded() || instance.ComposeService() != ""
}

func (gui *Gui) renderInstanceConfig(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceConfigStr(instance) })
}

// instanceConfigStr is the dump alone: what used to head it - name, type,
// status, created, profiles - is the Info tab's identity block now.
func (gui *Gui) instanceConfigStr(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok {
		return gui.Tr.WaitingForInstanceInfo
	}

	data, err := utils.MarshalIntoYaml(full)
	if err != nil {
		return fmt.Sprintf("Error marshalling instance details: %v", err)
	}

	return utils.ColoredYamlString(string(data))
}

func (gui *Gui) refreshInstances() error {
	if gui.Views.Instances == nil {
		return nil
	}

	instances, err := gui.IncusCommand.GetInstances(gui.Panels.Instances.List.GetAllItems())
	if err != nil {
		return err
	}

	// Computed over what the panel will actually show: with the local stack
	// gone to the services panel, the instances left can sit in one project
	// even when the server's don't.
	standalone := lo.Reject(instances, func(instance *commands.Instance, _ int) bool {
		return gui.isLocalComposeInstance(instance)
	})

	gui.State.SpansProjects.Instances = spansMultipleProjects(
		lo.Map(standalone, func(instance *commands.Instance, _ int) string { return instance.Project }))

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
			err := inst.Delete()
			if errors.Is(err, commands.ErrInstanceRunning) {
				return gui.promptToForceDeleteInstance(inst)
			}

			if err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshInstances()
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

			return gui.refreshInstances()
		})
	}, nil)
}

// handleInstanceCopyIPv4 copies the selected instance's IPv4 address to the
// system clipboard. An instance can have several (one per interface); we copy
// the first, which is the address people generally want to paste somewhere.
func (gui *Gui) handleInstanceCopyIPv4(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.instanceCopyIPv4(inst)
}

func (gui *Gui) instanceCopyIPv4(inst *commands.Instance) error {
	addresses := inst.Addresses("inet")
	if len(addresses) == 0 {
		return gui.createErrorPanel(gui.Tr.NoIPv4Address)
	}

	address := addresses[0]
	if err := gui.OSCommand.CopyToClipboard(address); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	gui.WithTransientStatus(fmt.Sprintf("%s %s", gui.Tr.CopiedToClipboard, address), time.Second*2)

	return nil
}

func (gui *Gui) handleInstanceViewLogs(g *gocui.Gui, v *gocui.View) error {
	if err := gui.Panels.Instances.SetMainTab("logs"); err != nil {
		return err
	}

	return gui.handleEnterMain(g, v)
}

func (gui *Gui) handleInstancesExecShell(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.instanceExecShell(inst)
}

// instanceCLIArgs carries the instance's project through to the `incus` CLI,
// which otherwise uses whatever project the user's own remote is set to -
// not necessarily the one the selected instance lives in.
func instanceCLIArgs(instance *commands.Instance) []string {
	if instance.Project == "" {
		return nil
	}

	return []string{"--project", instance.Project}
}

func (gui *Gui) handleInstanceAttach(g *gocui.Gui, v *gocui.View) error {
	inst, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.instanceAttachConsole(inst)
}

// instanceAttachConsole shells out to `incus console`, the analog of
// lazydocker's `docker attach`: it hands the terminal to the instance's
// console rather than starting a process in it the way exec does.
func (gui *Gui) instanceAttachConsole(instance *commands.Instance) error {
	if !instance.IsRunning() {
		return gui.createErrorPanel(gui.Tr.CannotAttachStoppedInstanceError)
	}

	cmd := gui.OSCommand.NewCmd("incus", append(instanceCLIArgs(instance), "console", instance.Name)...)

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

	args := append(instanceCLIArgs(instance), "exec", instance.Name, "--", "sh", "-c",
		"exec $(command -v bash || command -v ash || command -v sh)")

	cmd := gui.OSCommand.NewCmd("incus", args...)

	return gui.runSubprocessWithMessage(cmd, gui.Tr.ExitShellToReturn)
}
