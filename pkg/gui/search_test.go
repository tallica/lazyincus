package gui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

func (s *screen) searchMain(t *testing.T, text string) {
	t.Helper()

	s.pressKey(t, tcell.KeyEnter)
	s.press(t, '/')
	s.settle(t, "search:")
	s.typeText(t, text)
}

func TestSearchingTheMainPanelStepsThroughItsMatches(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.searchMain(t, "packets")
	s.settle(t, "search (1 of 2): packets")

	s.pressKey(t, tcell.KeyEnter)
	s.press(t, 'n')
	s.settle(t, "search (2 of 2): packets")
	assert.Equal(t, "main", onLoop(t, s, s.gui.currentViewName))

	s.press(t, 'n')
	s.settle(t, "search (1 of 2): packets")
	s.press(t, 'N')
	s.settle(t, "search (2 of 2): packets")

	// The first esc clears the search, the second leaves the main panel.
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "PgUp/PgDn: scroll")
	assert.Equal(t, "main", onLoop(t, s, s.gui.currentViewName))
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "PgUp/PgDn: scroll")
	assert.Equal(t, "instances", onLoop(t, s, s.gui.currentViewName))
}

func TestASearchWithNoMatchesSaysSo(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.searchMain(t, "nowhere")
	s.settle(t, "search (no matches): nowhere")
}

func TestLeavingTheMainPanelEndsItsSearch(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.searchMain(t, "packets")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "search (1 of 2): packets")

	s.press(t, '1')
	s.settle(t, "PgUp/PgDn: scroll")
	assert.False(t, onLoop(t, s, s.gui.Views.Main.IsSearching))
}
