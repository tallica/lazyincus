package gui

import (
	"os/exec"

	"github.com/jesseduffield/gocui"
	"github.com/mgutz/str"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
)

// handleTypedCommand asks for an incus command line, seeded with the project
// of view's selected row, and runs it with the terminal handed over, on the
// remote view's keys act on. Split on spaces outside quotes and never run
// through a shell: an unconfirmed prompt takes no pipes or $(...).
func (gui *Gui) handleTypedCommand(view string) func(*gocui.Gui, *gocui.View) error {
	return func(*gocui.Gui, *gocui.View) error {
		remote := gui.actionRemote(view)

		initial := ""
		if project := gui.typedCommandProject(view); project != "" {
			initial = "--project " + project + " "
		}

		return gui.openTextPrompt(gui.onRemote(gui.Tr.TypedCommandTitle, remote), gui.Tr.TypedCommandHint, initial, func(line string) error {
			cmd := gui.typedCommandCmd(line, remote)
			if cmd == nil {
				return nil
			}

			if err := gui.runSubprocess(cmd); err != nil {
				return err
			}

			gui.refreshInBackground(lo.Flatten(gui.fetchGroups())...)

			return nil
		})
	}
}

// typedCommandCmd is line as an incus command on remote, a leading "incus"
// typed out of habit dropped; nil for nothing to run.
func (gui *Gui) typedCommandCmd(line, remote string) *exec.Cmd {
	args := str.ToArgv(line)
	if len(args) > 0 && args[0] == "incus" {
		args = args[1:]
	}

	if len(args) == 0 {
		return nil
	}

	return commands.WithRemote(gui.OSCommand.NewCmd("incus", args...), remote)
}

// typedCommandProject is the project of view's selected row, or the one the
// panels are scoped to when the row names none.
func (gui *Gui) typedCommandProject(view string) string {
	project := ""

	switch view {
	case "instances":
		if instance, err := gui.Panels.Instances.GetSelectedItem(); err == nil {
			project = instance.Project
		}
	case "services":
		if row, err := gui.Panels.Services.GetSelectedItem(); err == nil {
			project = row.Service.Project
		}
	case "stacks", "backups":
		if stack := gui.selectedStack.Load(); stack != nil {
			project = stack.Name
		}
	case "snapshots":
		if snapshot, err := gui.Panels.Snapshots.GetSelectedItem(); err == nil {
			project = snapshot.Project
		}
	case "images":
		if image, err := gui.Panels.Images.GetSelectedItem(); err == nil {
			project = image.Image.Project
		}
	case "volumes":
		if volume, err := gui.Panels.Volumes.GetSelectedItem(); err == nil {
			project = volume.Volume.Project
		}
	case "networks":
		if network, err := gui.Panels.Networks.GetSelectedItem(); err == nil {
			project = network.Network.Project
		}
	case "profiles":
		if profile, err := gui.Panels.Profiles.GetSelectedItem(); err == nil {
			project = profile.Profile.Project
		}
	}

	if project == "" && !gui.IncusCommand.IsAllProjects() {
		project = gui.IncusCommand.ProjectName()
	}

	return project
}
