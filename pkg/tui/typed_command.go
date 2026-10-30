package tui

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

// typedCommand: never through a shell.
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
	m.bindPromptCompletion(m.incusCompleter(remote, seed))
	m.bindPromptHistory(config.IncusHistory, initial)

	return nil
}

// typedComposeCommand: a service's name is offered after the cursor, for the verb typed before it.
func typedComposeCommand(m *Model, target composeTarget) tea.Cmd {
	initial := ""
	if target.service != "" {
		initial = " " + target.service
	}

	title := m.onRemote(fmt.Sprintf(m.tr.TypedComposeCommandTitle, target.project), target.remote)
	m.textPrompt(title, m.tr.TypedCommandHint, initial, typedLineCheck(initial), func(m *Model, line string) tea.Cmd {
		cmd, err := m.typedComposeCmd(line, target)
		if err != nil {
			m.openError(err.Error())
			return nil
		}

		return m.runTyped(cmd, line, config.ComposeHistory, target.remote, &target)
	})
	m.topPrompt().input.CursorStart()
	m.bindPromptCompletion(m.composeCompleter(target))
	m.bindPromptHistory(config.ComposeHistory, initial)

	return nil
}

// bindPromptHistory: beyond initial, only lines starting with the cursor's left are stepped to.
func (m *Model) bindPromptHistory(history config.History, initial string) {
	lines, err := m.cfg.LoadHistory(history)
	if err != nil {
		m.incus.Log.Warn().Err(err).Msg("reading command history")

		return
	}

	p := m.topPrompt()
	at, draft, prefix, tail := len(lines), "", "", ""

	step := func(by int) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			if at == len(lines) {
				draft = p.input.Value()
				prefix, tail = historyPrefix(p.input, draft, initial)
			}

			next := at + by
			for next >= 0 && next < len(lines) && (!strings.HasPrefix(lines[next], prefix) || lines[next] == draft) {
				next += by
			}

			if next < 0 || (next > len(lines) && at == len(lines)) {
				return nil
			}

			at = min(next, len(lines))
			text := draft
			if at < len(lines) {
				text = lines[at]
			}

			p.input.SetValue(text)
			p.input.SetCursor(len([]rune(text)))
			p.subtitle = m.tr.TypedCommandHint
			// The draft's cursor where it was: before a service on ;.
			if at == len(lines) {
				p.input.SetCursor(len([]rune(draft)) - len([]rune(tail)))
			}

			return nil
		}
	}

	for _, b := range []struct {
		key string
		by  int
	}{{"up", -1}, {"down", 1}} {
		p.bindings = append(p.bindings, &binding{view: confirmationView, key: b.key, handler: step(b.by)})
	}
}

// historyPrefix is what a line must start with to be stepped to, and the draft after the cursor.
func historyPrefix(input *textinput.Model, draft, initial string) (string, string) {
	before, after := splitAtCursor(input, draft)

	if typed := strings.TrimSpace(before); typed == "" || typed == strings.TrimSpace(initial) {
		return "", after
	}

	return before, after
}

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

// typedComposeCmd is line in target's stack; no seed, the service is offered after the line.
func (m *Model) typedComposeCmd(line string, target composeTarget) (*exec.Cmd, error) {
	args, err := typedArgs(line, nil, "incus-compose")
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}

	return commands.WithRemote(m.incus.ComposeCmd(target.dir, args...), target.remote), nil
}

// typedCommandProject: the row's, else the session's on its own remote.
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
