package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/config"
)

func typeText(m *Model, text string) {
	for _, r := range text {
		if r == ' ' {
			press(m, "space")
			continue
		}
		press(m, string(r))
	}
}

// The test model lists cache, web (running) and db (stopped).

func TestFilteringAListKeepsTheRowsWithTheNeedle(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "web")
	assert.Contains(t, footer(m), "filter: web")
	assert.Equal(t, 1, m.panelNamed("instances").count())
	assert.Equal(t, "web", m.selectedInstance().Name, "main follows what's left")

	press(m, "enter")
	assert.Equal(t, "instances", m.currentView())
	assert.Contains(t, footer(m), "filter: web", "the filter stays once the prompt is left")

	press(m, "esc")
	assert.Equal(t, 3, m.panelNamed("instances").count())
	assert.Contains(t, footer(m), "PgUp/PgDn: scroll")
}

func TestTheFilterMatchesAsTyped(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "Web")
	assert.Equal(t, 0, m.panelNamed("instances").count())
	assert.Nil(t, m.selectedInstance())
}

func TestTheSelectionFollowsItsItemThroughTheFilter(t *testing.T) {
	m := start(t, options{}, 140, 40)
	press(m, "down")
	require.Equal(t, "web", m.selectedInstance().Name)

	press(m, "/")
	typeText(m, "e")
	assert.Equal(t, "web", m.selectedInstance().Name)

	press(m, "up")
	assert.Equal(t, "cache", m.selectedInstance().Name, "up and down move the list being filtered")
	assert.Equal(t, filterView, m.currentView())
}

func TestEscInTheFilterPromptDropsTheFilter(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "db")
	press(m, "esc")
	assert.Equal(t, "instances", m.currentView())
	assert.Equal(t, 3, m.panelNamed("instances").count())
	assert.False(t, m.filter.active)
}

func TestReopeningTheFilterKeepsWhatWasTyped(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "we")
	press(m, "enter", "/", "b")
	assert.Contains(t, footer(m), "filter: web")
}

func TestARefreshKeepsTheFilter(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "web")
	refreshInstances(m)
	assert.Equal(t, 1, m.panelNamed("instances").count())
}

// pkg/gui's TestTabInAFilterCyclesPanels.
func TestTabInAFilterCyclesPanels(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "web")
	press(m, "tab")
	assert.Equal(t, "snapshots", m.currentView())
	assert.Empty(t, m.filter.panel)
	assert.Equal(t, 3, m.panelNamed("instances").count())
}

func TestSearchingMainDropsAListFilter(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "/")
	typeText(m, "web")
	press(m, "enter", "enter", "/")
	assert.Contains(t, footer(m), "search: ")
	assert.Equal(t, 3, m.panelNamed("instances").count())
}

func TestIgnoredStringsHideRows(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) { c.Ignore = []string{"cache"} }}, 140, 40)

	assert.Equal(t, 2, m.panelNamed("instances").count())
	assert.NotEqual(t, "cache", m.selectedInstance().Name)
}
