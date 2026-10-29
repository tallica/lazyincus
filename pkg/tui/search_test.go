package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

// The pkg/gui search tests, on the Config tab: Info has no "packets" yet.

func searchMain(m *Model, text string) {
	press(m, "]", "enter", "/")
	for _, r := range text {
		press(m, string(r))
	}
}

func footer(m *Model) string {
	return screen(m)[m.height-1]
}

func TestSearchingTheMainPanelStepsThroughItsMatches(t *testing.T) {
	m := start(t, options{}, 140, 40)

	searchMain(m, "false")
	assert.Contains(t, footer(m), "search (1 of 2): false")

	press(m, "enter", "n")
	assert.Contains(t, footer(m), "search (2 of 2): false")
	assert.Equal(t, mainView, m.currentView())

	press(m, "n")
	assert.Contains(t, footer(m), "search (1 of 2): false")
	press(m, "N")
	assert.Contains(t, footer(m), "search (2 of 2): false")

	// The first esc clears the search, the second leaves the main panel.
	press(m, "esc")
	assert.Contains(t, footer(m), "PgUp/PgDn: scroll")
	assert.Equal(t, mainView, m.currentView())
	press(m, "esc")
	assert.Contains(t, footer(m), "PgUp/PgDn: scroll")
	assert.Equal(t, "instances", m.currentView())
}

func TestASearchWithNoMatchesSaysSo(t *testing.T) {
	m := start(t, options{}, 140, 40)

	searchMain(m, "nowhere")
	assert.Contains(t, footer(m), "search (no matches): nowhere")
}

func TestLeavingTheMainPanelEndsItsSearch(t *testing.T) {
	m := start(t, options{}, 140, 40)

	searchMain(m, "false")
	press(m, "enter")
	assert.Contains(t, footer(m), "search (1 of 2): false")

	press(m, "1")
	assert.Contains(t, footer(m), "PgUp/PgDn: scroll")
	assert.False(t, m.main.searching())
}

func TestTheSearchPromptTypesWhatWouldBeKeys(t *testing.T) {
	m := start(t, options{}, 140, 40)

	searchMain(m, "q1nJ")
	assert.Contains(t, footer(m), "search (no matches): q1nJ")
	assert.Equal(t, filterView, m.currentView())

	press(m, "esc")
	assert.Equal(t, mainView, m.currentView())
	assert.Contains(t, footer(m), "PgUp/PgDn: scroll")
}

func TestMatchesAreHighlighted(t *testing.T) {
	m := start(t, options{}, 140, 40)
	searchMain(m, "false")

	view := m.View().Content
	assert.Contains(t, view, currentMatchStyle+"f")
	assert.Contains(t, view, matchStyle+"f")
}

func TestSearchIsCaseSensitiveOnlyWithACapital(t *testing.T) {
	var p pane
	p.set("k", "Error\nerror\nERROR", false, false, 20, 4)

	p.find("error")
	assert.Len(t, p.search.matches, 3)
	p.find("Error")
	assert.Equal(t, []match{{0, 5, 0}}, p.search.matches)
}

func TestSearchNeverMatchesAcrossAWrap(t *testing.T) {
	var p pane
	p.set("k", "abcdef", true, false, 3, 4)

	p.find("cd")
	assert.Empty(t, p.search.matches)
}

func TestAMatchOutOfViewComesToTheMiddle(t *testing.T) {
	var p pane
	p.set("k", numbered(20)+"\nneedle\n"+numbered(20), false, false, 20, 6)

	p.find("needle")
	assert.Equal(t, 17, p.oy)
	assert.Equal(t, 3, p.cy)
}

func TestNewContentKeepsTheSearchAndMovesOffAGoneMatch(t *testing.T) {
	var p pane
	p.set("a", "x\nx\nx", false, false, 20, 4)
	p.find("x")
	p.nextMatch(1)
	p.nextMatch(1)
	assert.Equal(t, 2, p.search.current)

	p.set("b", "x\ny", false, false, 20, 4)
	assert.Equal(t, "x", p.search.needle, "a new item keeps the search")
	assert.Equal(t, 0, p.search.current, "the nearest to the cursor, back at the top")
}

func TestTheSearchPromptTakesTheFooter(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) { c.Gui.ShowBottomLine = false }}, 140, 40)
	assert.NotContains(t, footer(m), "PgUp/PgDn")

	searchMain(m, "fals")
	assert.Equal(t, "search (1 of 2): fals", strings.TrimRight(footer(m), " "))

	cursor := m.View().Cursor
	assert.Equal(t, len("search (1 of 2): fals"), cursor.X)
	assert.Equal(t, 39, cursor.Y)

	press(m, "left")
	assert.Equal(t, len("search (1 of 2): fal"), m.View().Cursor.X)
}

func TestKeysTheEditorDoesntTakeStillWork(t *testing.T) {
	m := start(t, options{}, 90, 9)
	searchMain(m, "!")

	press(m, "pgdown")
	assert.Equal(t, m.cfg.UserConfig.Gui.ScrollHeight, m.main.oy)
	assert.Equal(t, filterView, m.currentView())
}
