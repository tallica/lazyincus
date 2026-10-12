package gui

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
)

// handleTypedCommand asks for an incus command line, seeded with the project
// of view's selected row, and runs it with the terminal handed over, on the
// remote view's keys act on. Never through a shell: an unconfirmed prompt
// takes no pipes or $(...).
func (gui *Gui) handleTypedCommand(view string) func(*gocui.Gui, *gocui.View) error {
	return func(*gocui.Gui, *gocui.View) error {
		remote := gui.actionRemote(view)

		initial := ""
		if project := gui.typedCommandProject(view); project != "" {
			initial = "--project " + project + " "
		}

		return gui.textPrompt(
			gui.onRemote(gui.Tr.TypedCommandTitle, remote),
			gui.Tr.TypedCommandHint,
			initial,
			typedLineCheck(initial),
			func(line string) error {
				cmd, err := gui.typedCommandCmd(line, remote)
				if err != nil {
					return err
				}

				return gui.runTyped(cmd, "")
			},
		)
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
	if err := gui.textPrompt(
		title,
		gui.Tr.TypedCommandHint,
		initial,
		typedLineCheck(initial),
		func(line string) error {
			cmd, err := gui.typedComposeCmd(line, target)
			if err != nil {
				return err
			}

			return gui.runTyped(cmd, target.dir)
		},
	); err != nil {
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

// runTyped runs a typed command, nil for nothing to run, and re-reads the
// panels after: it can have changed anything. stackDir drops a stack's cached
// parse first - an incus-compose command can have changed what it declares -
// and is "" for the incus one.
func (gui *Gui) runTyped(cmd *exec.Cmd, stackDir string) error {
	if cmd == nil {
		return nil
	}

	if err := gui.runSubprocess(cmd); err != nil {
		return err
	}

	if stackDir != "" {
		gui.stacks.forget(stackDir)
	}

	gui.refreshAllInBackground()

	return nil
}

// typedLineCheck refuses enter on the seed the prompt started from, which is
// nothing typed, and a line that doesn't split - it would run other arguments
// than were typed.
func typedLineCheck(initial string) func(string) error {
	return func(text string) error {
		if text == strings.TrimSpace(initial) {
			return errKeepPrompt
		}

		_, err := splitArgs(text)

		return err
	}
}

// typedArgs splits line, dropping a leading binary typed out of habit.
func typedArgs(line, binary string) ([]string, error) {
	args, err := splitArgs(line)
	if err != nil {
		return nil, err
	}

	if len(args) > 0 && args[0] == binary {
		args = args[1:]
	}

	return args, nil
}

// splitArgs splits line on whitespace outside quotes, dropping the quote
// delimiters. A backslash is an ordinary character, so an argument holding
// one - a sed script, a path - keeps it. An unclosed quote is an error rather
// than the rest of the line run as one argument.
func splitArgs(line string) ([]string, error) {
	var (
		args    []string
		current strings.Builder
		quote   rune
		started bool
	)

	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote, started = r, true
		case unicode.IsSpace(r):
			if started {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}

	if started {
		args = append(args, current.String())
	}

	return args, nil
}

// typedCommandCmd is line as an incus command on remote; nil for nothing to
// run.
func (gui *Gui) typedCommandCmd(line, remote string) (*exec.Cmd, error) {
	args, err := typedArgs(line, "incus")
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}

	return commands.WithRemote(gui.OSCommand.NewCmd("incus", args...), remote), nil
}

// typedComposeCmd is line as an incus-compose command in target's stack; nil
// for nothing to run.
func (gui *Gui) typedComposeCmd(line string, target composeTarget) (*exec.Cmd, error) {
	args, err := typedArgs(line, "incus-compose")
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}

	return commands.WithRemote(gui.IncusCommand.ComposeCmd(target.dir, args...), target.remote), nil
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
