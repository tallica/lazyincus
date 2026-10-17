package gui

import (
	"slices"
	"strings"
	"unicode"

	"github.com/jesseduffield/gocui"
	"github.com/rivo/uniseg"
	"github.com/tallica/lazyincus/pkg/commands"
)

// completer answers what can follow args at word, the word before the cursor.
// It's asked off the main loop: incus's asks the CLI, and that the daemon.
type completer func(args []string, word string) (commands.Completion, error)

// bindPromptCompletion makes tab in the open prompt complete the word before
// the cursor: one candidate replaces it, several extend it as far as they
// agree and are listed where the hint was.
func (gui *Gui) bindPromptCompletion(complete completer) error {
	view := gui.Views.Confirmation

	return gui.g.SetKeybinding("confirmation", gocui.KeyTab, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
		content := view.TextArea.GetUnwrappedContent()

		before, after, ok := splitAtCursor(view.TextArea, content)
		if !ok {
			return nil
		}

		args, word, ok := completionWord(before)
		if !ok {
			return nil
		}

		go func() {
			completion, err := complete(args, word)
			if err != nil {
				gui.Log.Warn().Err(err).Msg("completing a typed command")

				return
			}

			gui.g.Update(func(*gocui.Gui) error {
				// Typed on, or closed, while the CLI was asked.
				if !view.Visible || !view.Editable || view.TextArea.GetUnwrappedContent() != content {
					return nil
				}

				gui.applyCompletion(view, strings.TrimSuffix(before, word), word, after, completion)

				return nil
			})
		}()

		return nil
	})
}

func (gui *Gui) applyCompletion(view *gocui.View, head, word, after string, completion commands.Completion) {
	candidates := completion.Candidates
	if len(candidates) == 0 {
		view.Subtitle = gui.Tr.NoCompletions

		return
	}

	completed := commonPrefix(candidates)
	if len(candidates) == 1 && !completion.NoSpace && !strings.HasPrefix(after, " ") {
		completed += " "
	}

	if len(candidates) > 1 {
		// Fitted to the popup as it stands: a list long enough to widen it
		// would have it jump on every tab.
		x0, _, x1, _ := view.Dimensions()
		view.Subtitle = fitBeside(strings.Join(candidates, " "), view.Title, x1-x0)
	} else {
		view.Subtitle = gui.Tr.TypedCommandHint
	}

	if len(completed) <= len(word) {
		return
	}

	view.ClearTextArea()
	view.TextArea.TypeString(head + completed + after)

	for range uniseg.GraphemeClusterCount(after) {
		view.TextArea.MoveCursorLeft()
	}

	view.RenderTextArea()
}

// splitAtCursor is content either side of the text area's cursor, gocui
// giving it only as a screen column: false for a line the prompt has wrapped.
func splitAtCursor(textArea *gocui.TextArea, content string) (string, string, bool) {
	x, y := textArea.GetCursorXY()
	if y != 0 {
		return "", "", false
	}

	at, width := 0, 0
	graphemes := uniseg.NewGraphemes(content)

	for width < x && graphemes.Next() {
		width += graphemes.Width()
		_, at = graphemes.Positions()
	}

	return content[:at], content[at:], true
}

// completionWord is before split into the arguments it has finished and the
// word it ends in, "" after a space; false for an unclosed quote, where the
// word's start is anybody's guess.
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

// composeCompleter offers incus-compose's verbs for the first word and the
// stack's services after it.
func (gui *Gui) composeCompleter(target composeTarget) completer {
	return func(args []string, word string) (commands.Completion, error) {
		args = dropBinary(args, nil, "incus-compose")

		options := commands.ComposeVerbs
		if slices.ContainsFunc(args, func(arg string) bool { return !strings.HasPrefix(arg, "-") }) {
			options = nil
			for _, service := range gui.stacks.get(target.dir, false, gui.loadStack).Services {
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

// incusCompleter asks the incus CLI on remote, the binary typed out of habit
// dropped.
func (gui *Gui) incusCompleter(remote string, seed []string) completer {
	return func(args []string, word string) (commands.Completion, error) {
		return gui.completeIncus(remote, dropBinary(args, seed, "incus"), word)
	}
}
