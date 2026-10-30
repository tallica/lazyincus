package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// menuRows is the text inside the top popup's frame, right-trimmed.
func menuRows(m *Model, lines []string) []string {
	x0, y0, x1, y1, _ := m.popupLayout(m.popups[len(m.popups)-1])

	rows := []string{}
	for y := y0 + 1; y < y1; y++ {
		rows = append(rows, strings.TrimRight(ansi.Cut(lines[y], x0+1, x1), " "))
	}

	return rows
}

func TestTheOptionsMenuRunsTheKeyPicked(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "x")
	require.Equal(t, menuView, m.currentView())
	for m.topMenu().selectedItem().labelColumns[0] != "+" {
		press(m, "j")
	}
	press(m, "enter")

	assert.Empty(t, m.popups)
	assert.Equal(t, screenHalf, m.screenMode)
}

func TestTheOptionsMenuClosesOnEscAndQ(t *testing.T) {
	for _, key := range []string{"esc", "q"} {
		m := start(t, options{}, 140, 40)
		press(m, "x", key)
		assert.Empty(t, m.popups, key)
	}
}

func TestTheOptionsMenuDoesntOpenOverAPopup(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "x", "?")
	assert.Len(t, m.popups, 1)
}

func TestTheOptionsMenuFromMainListsItsPanelsKeysToo(t *testing.T) {
	m := start(t, options{}, 140, 40)
	press(m, "enter", "x")

	labels := make([]string, 0, len(m.topMenu().items))
	for _, item := range m.topMenu().items {
		labels = append(labels, item.labelColumns[0]+" "+item.labelColumns[1])
	}

	assert.Contains(t, labels, "E exec shell")
	assert.Contains(t, labels, "/ "+m.tr.LcSearch)
	assert.NotContains(t, labels, "/ filter list", "main's own / hides its panel's")
}

func TestAMenuGetsACancelRow(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openMenu(menuOptions{title: "Pick", items: []*menuItem{{label: "one"}}})

	assert.Len(t, m.topMenu().items, 2)
	assert.Equal(t, "cancel", m.topMenu().items[1].labelColumns[0])
}

func TestAChecklistTicksAndHandsOverWhatsTicked(t *testing.T) {
	m := start(t, options{}, 140, 40)
	var picked []string
	m.openMenu(menuOptions{
		title: "Pick",
		items: []*menuItem{{label: "one"}, {label: "two"}, {label: "three"}},
		onConfirm: func(items []*menuItem) tea.Cmd {
			for _, item := range items {
				picked = append(picked, item.labelColumns[0])
			}
			return nil
		},
	})
	require.Len(t, m.topMenu().items, 3, "a checklist has no cancel row")

	press(m, "space")
	assert.Contains(t, menuRows(m, screen(m))[0], "[x] one")
	press(m, "j", "j", "space", "enter")

	assert.Equal(t, []string{"one", "three"}, picked)
	assert.Empty(t, m.popups)
}

func TestAChecklistTicksAllThenNone(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openMenu(menuOptions{
		title: "Pick", items: []*menuItem{{label: "one"}, {label: "two"}},
		onConfirm: func([]*menuItem) tea.Cmd { return nil },
	})

	press(m, "a")
	assert.True(t, m.topMenu().items[0].checked && m.topMenu().items[1].checked)
	assert.Contains(t, screen(m)[39], "a: tick all / none")
	press(m, "a")
	assert.False(t, m.topMenu().items[0].checked || m.topMenu().items[1].checked)
}

func TestAMenuHidesWhatWaitsForAFilter(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openMenu(menuOptions{title: "Pick", hideCancel: true, items: []*menuItem{{label: "one"}, {label: "hidden", hideUntilFiltered: true}}})

	assert.Equal(t, 1, m.topMenu().list.count())
}

func TestAMenuKeyThatMutatesIsRefusedReadOnly(t *testing.T) {
	m := start(t, options{readOnly: true}, 140, 40)
	ran := false
	m.openMenu(menuOptions{title: "Pick", items: []*menuItem{{
		label: "one",
		keys:  map[string]menuKey{"d": {mutates: true, handler: func(*Model) tea.Cmd { ran = true; return nil }}},
	}}})

	press(m, "d")
	assert.False(t, ran)
	assert.Contains(t, strings.Join(screen(m), "\n"), "--read-only")
}

func TestALongMenuScrollsToItsSelection(t *testing.T) {
	m := start(t, options{}, 140, 20)
	items := make([]*menuItem, 40)
	for i := range items {
		items[i] = &menuItem{label: strings.Repeat("x", i+1)}
	}
	m.openMenu(menuOptions{title: "Pick", items: items})

	for range 30 {
		press(m, "j")
	}
	rows := menuRows(m, screen(m))
	require.Len(t, rows, 16, "a menu taller than the screen is cut to it")
	assert.Equal(t, strings.Repeat("x", 31), rows[len(rows)-1])

	press(m, "k")
	assert.Equal(t, 29, m.topMenu().list.selected)
}

func TestAWrappedErrorKeepsItsColourOnEveryLine(t *testing.T) {
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = true })

	m := start(t, options{}, 80, 24)
	m.openError(strings.Repeat("word ", 30))

	_, _, _, _, lines := m.popupLayout(m.popups[0])

	assert.Greater(t, len(lines), 1)
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "\x1b[31m"), "%q", line)
	}
}

func TestAMessageDrawsItsFirstLineBold(t *testing.T) {
	m := start(t, options{}, 80, 24)
	m.openMessage("Title", "hello\nworld")

	for _, line := range strings.Split(m.render(), "\n") {
		if strings.Contains(line, "hello") {
			assert.NotContains(t, line, "\x1b[mhello", "bold carries on from the border")
		}
		if strings.Contains(line, "world") {
			assert.Contains(t, line, "\x1b[mworld", "the second line is plain")
		}
	}
}

func TestAnErrorReplacesAnOpenConfirmation(t *testing.T) {
	m := start(t, options{}, 80, 24)
	m.openConfirmation("Confirm", "Sure?", func() tea.Cmd { return nil })
	m.openError("boom")

	assert.Len(t, m.popups, 1)
	press(m, "esc")
	assert.Empty(t, m.popups, "pkg/gui has one confirmation view: the question is gone")
}

func TestAPromptDrawsItsTextBold(t *testing.T) {
	m := start(t, options{}, 80, 24)
	m.openTextPrompt("Title", "hint", "typed", func(*Model, string) tea.Cmd { return nil })

	for _, line := range strings.Split(m.render(), "\n") {
		if strings.Contains(line, "typed") {
			assert.NotContains(t, line, "\x1b[mtyped", "bold carries on from the border")
		}
	}
}
