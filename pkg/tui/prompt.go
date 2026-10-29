package tui

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// errKeepPrompt is a check's answer that enter should do nothing.
var errKeepPrompt = errors.New("keep prompt")

func refuseEmpty(text string) error {
	if text == "" {
		return errKeepPrompt
	}

	return nil
}

func acceptEmpty(string) error { return nil }

// openTextPrompt is pkg/gui's: a line of text, from initial, for submit.
func (m *Model) openTextPrompt(title, hint, initial string, submit func(*Model, string) tea.Cmd) {
	m.textPrompt(title, hint, initial, refuseEmpty, submit)
}

// openOptionalTextPrompt is openTextPrompt where enter on nothing submits it.
func (m *Model) openOptionalTextPrompt(title, hint string, submit func(*Model, string) tea.Cmd) {
	m.textPrompt(title, hint, "", acceptEmpty, submit)
}

// textPrompt is pkg/gui's: check's errKeepPrompt leaves the prompt as it is,
// any other error leaves it up saying why in place of the hint.
func (m *Model) textPrompt(title, hint, initial string, check func(string) error, submit func(*Model, string) tea.Cmd) {
	input := newEditorInput()
	input.SetValue(initial)

	p := &popup{view: confirmationView, title: title, subtitle: hint, input: &input}
	p.bindings = []*binding{
		{view: confirmationView, key: "enter", handler: func(m *Model) tea.Cmd {
			text := strings.TrimSpace(p.input.Value())
			if err := check(text); err != nil {
				if !errors.Is(err, errKeepPrompt) {
					p.subtitle = err.Error()
				}
				return nil
			}

			m.closePopup()

			return submit(m, text)
		}},
		{view: confirmationView, key: "esc", handler: do((*Model).closePopup)},
	}

	m.popups = append(m.popups, p)
}

func (m *Model) topPrompt() *popup {
	if len(m.popups) == 0 || m.popups[len(m.popups)-1].input == nil {
		return nil
	}

	return m.popups[len(m.popups)-1]
}

func (m *Model) editPrompt(msg tea.Msg) tea.Cmd {
	p := m.topPrompt()

	var cmd tea.Cmd
	*p.input, cmd = p.input.Update(msg)

	return cmd
}

// promptCursor is where the cursor is in the prompt's text wrapped to width:
// gocui leaves it at the end of the first row, whatever the wrap.
func promptCursor(input *textinput.Model, width int) (row, col int) {
	cs := cells(input.Value())

	at, runes := 0, 0
	for at < len(cs) && runes < input.Position() {
		runes += len([]rune(cs[at].chr))
		at++
	}

	ranges := wrapRanges(cs, width)
	for row, r := range ranges {
		next := len(cs)
		if row < len(ranges)-1 {
			next = ranges[row+1][0]
		}
		// Before the next row's first cell, a space the wrap dropped included.
		if at < next || row == len(ranges)-1 {
			for _, c := range cs[r[0]:min(at, r[1])] {
				col += c.width
			}
			return row, min(col, max(width-1, 0))
		}
	}

	return 0, 0
}
