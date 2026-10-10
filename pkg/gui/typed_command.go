package gui

import (
	"fmt"
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
			return gui.runTyped(gui.typedCommandCmd(line, remote))
		})
	}
}

// typedComposeCommand is ; on Stacks and Services: an incus-compose command
// line for target's stack, run in its directory and on its remote. A
// service's name is offered after the cursor, for the verb typed before it.
func (gui *Gui) typedComposeCommand(target composeTarget) error {
	initial := ""
	if target.service != "" {
		initial = " " + target.service
	}

	title := gui.onRemote(fmt.Sprintf(gui.Tr.TypedComposeCommandTitle, target.project), target.remote)
	if err := gui.openTextPrompt(title, gui.Tr.TypedCommandHint, initial, func(line string) error {
		return gui.runTyped(gui.typedComposeCmd(line, target))
	}); err != nil {
		return err
	}

	gui.Views.Confirmation.TextArea.GoToStartOfLine()
	gui.Views.Confirmation.RenderTextArea()

	return nil
}

// handleTypedComposeCommand is ; on Services: the selected service's stack,
// a replica's row answering with its service.
func (gui *Gui) handleTypedComposeCommand(*gocui.Gui, *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.typedComposeCommand(serviceTarget(service))
}

// runTyped runs a typed command, nil for nothing to run, and re-reads every
// panel after: it can have changed anything.
func (gui *Gui) runTyped(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}

	if err := gui.runSubprocess(cmd); err != nil {
		return err
	}

	gui.refreshInBackground(lo.Flatten(gui.fetchGroups())...)

	return nil
}

// typedArgs splits line on spaces outside quotes, dropping a leading binary
// typed out of habit.
func typedArgs(line, binary string) []string {
	args := str.ToArgv(line)
	if len(args) > 0 && args[0] == binary {
		args = args[1:]
	}

	return args
}

// typedCommandCmd is line as an incus command on remote; nil for nothing
// to run.
func (gui *Gui) typedCommandCmd(line, remote string) *exec.Cmd {
	args := typedArgs(line, "incus")
	if len(args) == 0 {
		return nil
	}

	return commands.WithRemote(gui.OSCommand.NewCmd("incus", args...), remote)
}

// typedComposeCmd is line as an incus-compose command in target's stack;
// nil for nothing to run.
func (gui *Gui) typedComposeCmd(line string, target composeTarget) *exec.Cmd {
	args := typedArgs(line, "incus-compose")
	if len(args) == 0 {
		return nil
	}

	return commands.WithRemote(gui.IncusCommand.ComposeCmd(target.dir, args...), target.remote)
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
