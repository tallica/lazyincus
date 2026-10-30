package tui

import (
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// completer answers what can follow args at word, the word before the cursor; it runs off the loop.
type completer func(args []string, word string) (commands.Completion, error)

type completedMsg struct {
	prompt            *popup
	content           string
	head, word, after string
	completion        commands.Completion
	err               error
}

// bindPromptCompletion is pkg/gui's: tab completes the word before the cursor, and an edit puts the hint back.
func (m *Model) bindPromptCompletion(complete completer) {
	p := m.topPrompt()
	p.hint = m.tr.TypedCommandHint

	p.bindings = append(p.bindings, &binding{view: confirmationView, key: "tab", handler: func(m *Model) tea.Cmd {
		content := p.input.Value()
		before, after := splitAtCursor(p.input, content)

		args, word, ok := completionWord(before)
		if !ok {
			return nil
		}

		return func() tea.Msg {
			completion, err := complete(args, word)

			return completedMsg{
				prompt: p, content: content, head: strings.TrimSuffix(before, word), word: word, after: after,
				completion: completion, err: err,
			}
		}
	}})
}

func (m *Model) onCompleted(msg completedMsg) {
	if msg.err != nil {
		m.incus.Log.Warn().Err(msg.err).Msg("completing a typed command")

		return
	}

	// Typed on, or closed, while the CLI was asked.
	if m.topPrompt() != msg.prompt || msg.prompt.input.Value() != msg.content {
		return
	}

	m.applyCompletion(msg)
}

func (m *Model) applyCompletion(msg completedMsg) {
	p, completion := msg.prompt, msg.completion
	candidates := completion.Candidates
	if len(candidates) == 0 {
		p.subtitle = m.tr.NoCompletions

		return
	}

	completed := commonPrefix(candidates)
	if len(candidates) == 1 && !completion.NoSpace && !strings.HasPrefix(msg.after, " ") {
		completed += " "
	}

	if len(candidates) > 1 {
		// Fitted to the popup as it stands: a list long enough to widen it would have it jump on every tab.
		x0, x1 := m.popupX(p)
		p.subtitle = fitBeside(strings.Join(candidates, " "), p.title, x1-x0)
	} else {
		p.subtitle = m.tr.TypedCommandHint
	}

	if len(completed) <= len(msg.word) {
		return
	}

	p.input.SetValue(msg.head + completed + msg.after)
	p.input.SetCursor(len([]rune(msg.head + completed)))
}

func splitAtCursor(input *textinput.Model, content string) (string, string) {
	runes := []rune(content)
	at := min(input.Position(), len(runes))

	return string(runes[:at]), string(runes[at:])
}

// completionWord splits before into finished arguments and the word it ends in; false for an unclosed quote.
func completionWord(before string) ([]string, string, bool) {
	args, err := splitArgs(before)
	if err != nil {
		return nil, "", false
	}

	if before == "" || unicode.IsSpace(rune(before[len(before)-1])) || len(args) == 0 {
		return args, "", true
	}

	word := args[len(args)-1]
	if !strings.HasSuffix(before, word) {
		// Quoted: what's on the line isn't the word the CLI would be asked.
		return nil, "", false
	}

	return args[:len(args)-1], word, true
}

func commonPrefix(words []string) string {
	prefix := words[0]

	for _, word := range words[1:] {
		for !strings.HasPrefix(word, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}

	return prefix
}

// composeCompleter offers incus-compose's verbs for the first word and the stack's services after it.
func (m *Model) composeCompleter(target composeTarget) completer {
	cache, load := m.stackCache, m.loadStack

	return func(args []string, word string) (commands.Completion, error) {
		args = dropBinary(args, nil, "incus-compose")

		options := commands.ComposeVerbs
		if slices.ContainsFunc(args, func(arg string) bool { return !strings.HasPrefix(arg, "-") }) {
			options = nil
			for _, service := range cache.get(target.dir, false, load).Services {
				options = append(options, service.Name)
			}
		}

		var completion commands.Completion
		for _, option := range options {
			if strings.HasPrefix(option, word) {
				completion.Candidates = append(completion.Candidates, option)
			}
		}

		return completion, nil
	}
}

func (m *Model) incusCompleter(remote string, seed []string) completer {
	complete := m.completeIncus

	return func(args []string, word string) (commands.Completion, error) {
		return complete(remote, dropBinary(args, seed, "incus"), word)
	}
}
