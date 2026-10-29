package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openPrompt opens a required text prompt that records what it's handed.
func openPrompt(m *Model, initial string) *[]string {
	var submitted []string
	m.openTextPrompt("Rename db", "enter: rename", initial, func(_ *Model, text string) tea.Cmd {
		submitted = append(submitted, text)
		return nil
	})
	m.fitViews()

	return &submitted
}

func TestAPromptEditsFromWhereItStartsAndSubmitsTrimmed(t *testing.T) {
	m := start(t, options{}, 140, 40)
	submitted := openPrompt(m, "db")

	shown := strings.Join(screen(m), "\n")
	assert.Contains(t, shown, "Rename db")
	assert.Contains(t, shown, "enter: rename─────╮")
	assert.Contains(t, screen(m)[39], "enter: submit, esc: close")

	press(m, "backspace", "backspace")
	typeText(m, " cache ")
	press(m, "enter")

	assert.Equal(t, []string{"cache"}, *submitted)
	assert.Empty(t, m.popups)
}

func TestEnterOnNothingKeepsARequiredPrompt(t *testing.T) {
	m := start(t, options{}, 140, 40)
	submitted := openPrompt(m, "")

	typeText(m, "  ")
	press(m, "enter")

	assert.Empty(t, *submitted)
	require.Len(t, m.popups, 1)
	assert.Equal(t, "enter: rename", m.popups[0].subtitle)
}

func TestEnterOnNothingSubmitsAnOptionalPrompt(t *testing.T) {
	m := start(t, options{}, 140, 40)
	var submitted []string
	m.openOptionalTextPrompt("Back up", "enter: default name", func(_ *Model, text string) tea.Cmd {
		submitted = append(submitted, text)
		return nil
	})

	press(m, "enter")
	assert.Equal(t, []string{""}, submitted)
}

func TestACheckSaysWhyInPlaceOfTheHint(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.textPrompt("Prune", "a number", "", func(string) error { return errors.New("not a number") }, nil)

	typeText(m, "x")
	press(m, "enter")

	require.Len(t, m.popups, 1)
	assert.Contains(t, strings.Join(screen(m), "\n"), "not a number─────╮")
}

func TestEscDropsAPrompt(t *testing.T) {
	m := start(t, options{}, 140, 40)
	submitted := openPrompt(m, "db")

	press(m, "esc")
	assert.Empty(t, m.popups)
	assert.Empty(t, *submitted)
}

// gocui gives an editable view every key with a rune, and the global
// bindings only the others.
func TestAPromptTypesWhatWouldBeKeys(t *testing.T) {
	m := start(t, options{}, 140, 40)
	openPrompt(m, "")

	typeText(m, "qyn1x/")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "qyn1x/", m.popups[0].input.Value())

	press(m, "]")
	assert.Equal(t, 0, m.panelNamed("instances").mainTab)

	cmd := m.handleKey(keyPress("ctrl+c"))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestAPasteGoesIntoThePrompt(t *testing.T) {
	m := start(t, options{}, 140, 40)
	openPrompt(m, "")

	m.Update(tea.PasteMsg{Content: "web-2"})
	assert.Equal(t, "web-2", m.popups[0].input.Value())
}

func TestALongPromptWrapsAndItsCursorFollows(t *testing.T) {
	m := start(t, options{}, 140, 40)
	openPrompt(m, "")

	x0, y0, x1, y1, _ := m.popupLayout(m.popups[0])
	require.Equal(t, 1, y1-y0-1)

	words := strings.Repeat("word ", (x1-x0)/5+2)
	typeText(m, strings.TrimSpace(words))

	_, y0, _, y1, lines := m.popupLayout(m.popups[0])
	assert.Len(t, lines, 2)
	assert.Equal(t, 2, y1-y0-1, "the popup grows with the wrap")

	cursor := m.View().Cursor
	require.NotNil(t, cursor)
	assert.Equal(t, y0+2, cursor.Y, "on the second row")
	assert.Equal(t, x0+1+len(strings.TrimRight(lines[1], " ")), cursor.X)
}

func TestPromptCursorOnAWrap(t *testing.T) {
	input := newEditorInput()
	input.SetValue("abc def")

	for position, want := range map[int][2]int{0: {0, 0}, 3: {0, 3}, 4: {1, 0}, 7: {1, 3}} {
		input.SetCursor(position)
		row, col := promptCursor(&input, 5)
		assert.Equal(t, want, [2]int{row, col}, "position %d", position)
	}
}
