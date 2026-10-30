package tui

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

// typedCommand is pkg/gui's handleTypedCommand: never through a shell.
func (m *Model) typedCommand() tea.Cmd {
	// A global key reaches a popup that isn't editable too.
	if len(m.popups) > 0 {
		return nil
	}

	view := m.currentSideView()
	remote := m.actionRemote(view)

	seed := projectCLIArgs(m.typedCommandProject(view))
	initial := ""
	if len(seed) > 0 {
		initial = strings.Join(seed, " ") + " "
	}

	m.textPrompt(m.onRemote(m.tr.TypedCommandTitle, remote), m.tr.TypedCommandHint, initial, typedLineCheck(initial),
		func(m *Model, line string) tea.Cmd {
			cmd, err := m.typedCommandCmd(line, remote, seed)
			if err != nil {
				m.openError(err.Error())
				return nil
			}

			return m.runTyped(cmd, line, config.IncusHistory, remote, nil)
		})

	return nil
}

// runTyped is pkg/gui's.
func (m *Model) runTyped(cmd *exec.Cmd, line string, history config.History, remote string, stack *composeTarget) tea.Cmd {
	if cmd == nil {
		return nil
	}

	if err := m.cfg.RecordCommand(history, line); err != nil {
		m.incus.Log.Warn().Err(err).Msg("saving command history")
	}

	project := cliProject(cmd.Args[1:])
	if stack != nil {
		project = stack.project
	}

	return m.runWatched(cmd, remote, project, func(failures []daemonFailure) tea.Cmd {
		if stack != nil {
			m.stackCache.forget(stack.dir)
		}

		next := m.refreshAll()
		m.showFailures(failures, remote)

		return next
	})
}

// typedLineCheck refuses enter on the seed alone and on a line that doesn't split.
func typedLineCheck(initial string) func(string) error {
	return func(text string) error {
		if text == strings.TrimSpace(initial) {
			return errKeepPrompt
		}

		_, err := splitArgs(text)

		return err
	}
}

func typedArgs(line string, seed []string, binary string) ([]string, error) {
	args, err := splitArgs(line)
	if err != nil {
		return nil, err
	}

	return dropBinary(args, seed, binary), nil
}

// dropBinary is args without the binary typed out of habit after the seed.
func dropBinary(args, seed []string, binary string) []string {
	at := 0
	if len(seed) > 0 && len(args) >= len(seed) && slices.Equal(args[:len(seed)], seed) {
		at = len(seed)
	}

	if at < len(args) && args[at] == binary {
		args = slices.Delete(slices.Clone(args), at, at+1)
	}

	return args
}

// splitArgs splits on whitespace outside quotes; a backslash is ordinary, an unclosed quote an error.
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

func (m *Model) typedCommandCmd(line, remote string, seed []string) (*exec.Cmd, error) {
	args, err := typedArgs(line, seed, "incus")
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}

	return commands.WithRemote(m.os.NewCmd("incus", args...), remote), nil
}

// typedCommandProject is pkg/gui's: the row's, else the session's on its own remote.
func (m *Model) typedCommandProject(view string) string {
	if p := m.panelNamed(view); p != nil && p.project != nil {
		if project := p.project(); project != "" {
			return project
		}
	}

	if m.incus.IsAllProjects() {
		return ""
	}

	if m.actionRemote(view) != m.incus.RemoteName() {
		return ""
	}

	return m.incus.ProjectName()
}
