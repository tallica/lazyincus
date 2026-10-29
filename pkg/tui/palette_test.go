package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/config"
)

func openPaletteWith(m *Model, text string) string {
	press(m, "ctrl+p")
	typeText(m, text)

	return strings.Join(screen(m), "\n")
}

func TestThePaletteRunsTheFocusedPanelsAction(t *testing.T) {
	m := start(t, options{}, 140, 40)

	shown := openPaletteWith(m, "next tab")
	assert.Equal(t, filterView, m.currentView())
	assert.Regexp(t, `\] +Instances +next tab +cache`, shown)
	assert.Contains(t, screen(m)[39], "run: next tab")

	press(m, "enter")
	assert.Empty(t, m.popups)
	assert.Equal(t, 1, m.panelNamed("instances").mainTab)
	assert.Equal(t, "instances", m.currentView())
}

func TestThePaletteFocusesAnotherPanelsAction(t *testing.T) {
	m := start(t, options{}, 140, 40)

	openPaletteWith(m, "filter profiles")
	press(m, "enter")

	assert.Equal(t, filterView, m.currentView())
	assert.Equal(t, "profiles", m.filter.panel)
}

func TestEscClosesThePalette(t *testing.T) {
	m := start(t, options{}, 140, 40)
	before := m.currentView()

	openPaletteWith(m, "st")
	press(m, "esc")

	assert.Empty(t, m.popups)
	assert.Equal(t, before, m.currentView())
	assert.False(t, m.filter.active)
	assert.NotContains(t, strings.Join(screen(m), "\n"), "Commands")
}

func TestThePaletteHonoursReadOnly(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}}, 140, 40)

	assert.Regexp(t, `E +Instances +exec shell +cache read-only`, openPaletteWith(m, "exec shell"))

	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "fake is read-only in the config")
}

func TestThePaletteLeavesItselfOut(t *testing.T) {
	m := start(t, options{}, 140, 40)

	assert.NotContains(t, openPaletteWith(m, "palette"), "command palette")
}

func TestThePaletteListsItemsOnlyOnceYouType(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "ctrl+p")
	hidden := 0
	for _, item := range m.topMenu().items {
		if item.hideUntilFiltered {
			hidden++
		}
	}
	assert.Equal(t, 3, hidden, "a row per instance")
	assert.Equal(t, len(m.topMenu().items)-hidden, m.topMenu().list.count())

	typeText(m, "web")
	assert.Regexp(t, `Instances +web +default`, strings.Join(screen(m), "\n"))
}

func TestThePaletteGoesToAnItem(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "2")
	openPaletteWith(m, "db")
	press(m, "enter")

	assert.Equal(t, "instances", m.currentView())
	assert.Equal(t, "db", m.selectedInstance().Name)
}

func TestTabListsAnItemsActions(t *testing.T) {
	m := start(t, options{}, 140, 40)

	openPaletteWith(m, "db")
	press(m, "tab")

	shown := strings.Join(screen(m), "\n")
	assert.Contains(t, shown, "Commands: db")
	assert.Contains(t, shown, m.tr.CommandPaletteItemHint)
	assert.NotContains(t, shown, "tab:")
	assert.NotContains(t, shown, "next screen mode")
	assert.Equal(t, "db", m.selectedInstance().Name)

	typeText(m, "next tab")
	press(m, "enter")
	assert.Equal(t, 1, m.panelNamed("instances").mainTab)
}

func TestTabOnAnActionDoesNothing(t *testing.T) {
	m := start(t, options{}, 140, 40)

	openPaletteWith(m, "next tab")
	press(m, "tab")

	assert.Equal(t, filterView, m.currentView())
	assert.True(t, m.paletteOpen())
	assert.NotContains(t, strings.Join(screen(m), "\n"), "Commands:")
}

func TestEscFromAnItemsActionsLeavesItSelected(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "2")
	openPaletteWith(m, "db")
	press(m, "tab", "esc")

	assert.Empty(t, m.popups)
	assert.Equal(t, "instances", m.currentView())
	assert.False(t, m.paletteOpen())
	assert.Equal(t, "db", m.selectedInstance().Name)
}

func TestThePaletteWontActOnAnotherSelection(t *testing.T) {
	m := start(t, options{}, 140, 40)

	openPaletteWith(m, "exec shell")
	m.panelNamed("instances").selected = 1
	press(m, "enter")

	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "cache isn't selected any more")
}

// pkg/gui's palette is three quarters of the screen, its hint right-aligned
// on the top edge.
func TestThePaletteFrame(t *testing.T) {
	m := start(t, options{}, 140, 40)
	press(m, "ctrl+p")

	_, y0, _, _, _ := m.popupLayout(m.popups[0])
	top := screen(m)[y0]
	assert.Regexp(t, `^.{17}`+regexp.QuoteMeta("╭─Commands─"), top)
	assert.Contains(t, top, "─"+m.tr.CommandPaletteHint+"─────╮")
}

func TestThePaletteTakesOverFromASearch(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "enter", "/")
	typeText(m, "Name")
	press(m, "enter")
	openPaletteWith(m, "next tab")

	assert.False(t, m.filter.search)
	assert.False(t, m.main.searching())
	assert.Contains(t, screen(m)[39], "run: next tab")
	assert.Regexp(t, `\] +Instances +next tab`, strings.Join(screen(m), "\n"))
}
