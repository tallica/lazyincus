package gui

import (
	"regexp"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

func (s *screen) typeText(t *testing.T, text string) {
	t.Helper()

	for _, key := range text {
		s.press(t, key)
	}
}

func TestThePaletteRunsTheFocusedPanelsAction(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	selected := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Name
	})

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "stop")
	screen := s.settle(t, "run: stop")
	assert.Regexp(t, `s +Instances +stop +`+regexp.QuoteMeta(selected), screen)

	// A refresh rebuilds the rows; the one the palette named is still selected.
	require.NoError(t, s.gui.refreshInstances())
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Are you sure")
}

func TestThePaletteFocusesAnotherPanelsAction(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "filter profiles")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Devices - Config")

	assert.Equal(t, "filter", onLoop(t, s, s.gui.currentViewName))
	assert.Equal(t, "profiles", onLoop(t, s, func() string { return s.gui.State.Filter.panel.GetView().Name() }))
}

func TestEscClosesThePalette(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	before := onLoop(t, s, s.gui.currentViewName)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "st")
	s.pressKey(t, tcell.KeyEsc)

	assert.NotContains(t, s.settle(t, ""), "Commands")
	assert.Equal(t, before, onLoop(t, s, s.gui.currentViewName))
}

func TestThePaletteHonoursReadOnly(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	})
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "stop")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "fake is read-only")
}

// With no image there's nothing to delete, but pruning and filtering still
// make sense.
func TestThePaletteLeavesOutWhatNeedsASelection(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetImages(nil)
	s.server.Emit(incustest.Lifecycle("default", api.EventLifecycleImageDeleted, "/1.0/images/0123456789ab"))
	require.Eventually(t, func() bool {
		return onLoop(t, s, s.gui.Panels.Images.List.Len) == 0
	}, 5*time.Second, 10*time.Millisecond)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "images")
	screen := s.settle(t, "run: images")

	assert.Contains(t, screen, "prune unused images")
	assert.Regexp(t, `Images +filter list`, screen)
	assert.NotRegexp(t, `Images +delete`, screen)
}

func TestThePaletteListsItemsOnlyOnceYouType(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "tab: an item's actions")
	assert.False(t, onLoop(t, s, func() bool {
		return lo.SomeBy(s.gui.Panels.Menu.List.GetItems(), func(item *types.MenuItem) bool { return item.HideUntilFiltered })
	}))

	s.typeText(t, "incusbr0")
	assert.Regexp(t, `Networks +incusbr0`, s.settle(t, "run: incusbr0"))
}

// enter on an item focuses its panel with the item selected.
func TestThePaletteGoesToAnItem(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "incusbr0")
	s.settle(t, "run: incusbr0")
	s.pressKey(t, tcell.KeyEnter)

	require.Eventually(t, func() bool { return onLoop(t, s, s.gui.currentViewName) == "networks" }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, "incusbr0", onLoop(t, s, func() string {
		network, _ := s.gui.Panels.Networks.GetSelectedItem()
		return network.Name
	}))
}

// tab on an item lists only its own actions, which act on it.
func TestTabListsAnItemsActions(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "incusbr0")
	s.settle(t, "run: incusbr0")
	s.pressKey(t, tcell.KeyTab)

	screen := s.settle(t, "Commands: incusbr0")
	assert.NotContains(t, screen, "switch remote")
	assert.NotContains(t, screen, "tab:")
	assert.Equal(t, "networks", onLoop(t, s, s.gui.currentSideViewName))
	assert.Equal(t, "incusbr0", onLoop(t, s, func() string {
		network, _ := s.gui.Panels.Networks.GetSelectedItem()
		return network.Name
	}))

	s.typeText(t, "web")
	assert.False(t, onLoop(t, s, func() bool {
		return lo.SomeBy(s.gui.Panels.Menu.List.GetItems(), func(item *types.MenuItem) bool { return item.HideUntilFiltered })
	}))
	for range "web" {
		s.pressKey(t, tcell.KeyBackspace2)
	}

	s.typeText(t, "delete")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "incusbr0")
	assert.Equal(t, "confirmation", onLoop(t, s, s.gui.currentViewName))
}

func TestThePaletteLeavesItselfOut(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "palette")

	assert.NotContains(t, s.settle(t, "run: palette"), "command palette")
}

// An action has no actions of its own: tab leaves the palette as it was.
func TestTabOnAnActionDoesNothing(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "stop")
	s.settle(t, "run: stop")
	s.pressKey(t, tcell.KeyTab)

	screen := s.settle(t, "run: stop")
	assert.NotContains(t, screen, "Commands:")
	assert.Equal(t, "filter", onLoop(t, s, s.gui.currentViewName))
	assert.True(t, onLoop(t, s, s.gui.paletteOpen))
}

func TestEscFromAnItemsActionsLeavesItSelected(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "incusbr0")
	s.settle(t, "run: incusbr0")
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "Commands: incusbr0")
	s.pressKey(t, tcell.KeyEsc)

	assert.NotContains(t, s.settle(t, ""), "Commands")
	assert.Equal(t, "networks", onLoop(t, s, s.gui.currentViewName))
	assert.False(t, onLoop(t, s, s.gui.paletteOpen))
	assert.Equal(t, "incusbr0", onLoop(t, s, func() string {
		network, _ := s.gui.Panels.Networks.GetSelectedItem()
		return network.Name
	}))
}

// The palette's items are the lists as they were when it opened.
func TestTabOnAnItemGoneSinceSaysSo(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "db instances")
	s.settle(t, "run: db instances")

	s.removeInstance(t, "db")
	s.pressKey(t, tcell.KeyTab)

	screen := s.settle(t, "db isn't listed any more")
	assert.NotContains(t, screen, "Commands")

	s.pressKey(t, tcell.KeyEnter)
	assert.NotContains(t, s.settle(t, ""), "listed any more")
	assert.Equal(t, "instances", onLoop(t, s, s.gui.currentViewName))
	assert.False(t, onLoop(t, s, s.gui.paletteOpen))
}

// Outside the palette, tab in a filter still moves to the next panel.
func TestTabInAFilterCyclesPanels(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.press(t, '/')
	s.typeText(t, "web")
	s.settle(t, "filter: web")
	s.pressKey(t, tcell.KeyTab)

	require.Eventually(t, func() bool { return onLoop(t, s, s.gui.currentViewName) == "snapshots" }, 5*time.Second, 10*time.Millisecond)
	assert.Nil(t, onLoop(t, s, func() any { return s.gui.State.Filter.panel }))
	assert.Equal(t, 3, onLoop(t, s, s.gui.Panels.Instances.List.Len))
}

// removeInstance has the daemon drop name and waits for the list to follow.
func (s *screen) removeInstance(t *testing.T, name string) {
	t.Helper()

	s.server.SetInstances(lo.Reject(fixtureServer().Instances, func(instance api.InstanceFull, _ int) bool {
		return instance.Name == name
	}))
	require.NoError(t, s.gui.refreshInstances())
	require.Eventually(t, func() bool {
		return onLoop(t, s, s.gui.Panels.Instances.List.Len) == len(fixtureServer().Instances)-1
	}, 5*time.Second, 10*time.Millisecond)
}

// An action runs only on the item the palette named, not on whatever a
// refresh has moved the cursor to since.
func TestThePaletteWontActOnAnotherSelection(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	selected := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Name
	})

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "stop")
	s.settle(t, "run: stop")
	s.removeInstance(t, selected)
	s.pressKey(t, tcell.KeyEnter)

	screen := s.settle(t, selected+" isn't selected any more")
	assert.NotContains(t, screen, "Are you sure")
}

func TestAnItemsActionsWontActOnAnotherSelection(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "db instances")
	s.settle(t, "run: db instances")
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "Commands: db")
	s.typeText(t, "restart")
	s.settle(t, "run: restart")
	s.removeInstance(t, "db")
	s.pressKey(t, tcell.KeyEnter)

	screen := s.settle(t, "db isn't selected any more")
	assert.NotContains(t, screen, "Are you sure")
}

// A menu opened after the palette is the plain menu again: its width,
// prompt and border carry nothing of the palette's.
func TestAMenuAfterThePaletteIsAPlainMenu(t *testing.T) {
	s := startScreen(t, 90, 40, nil)
	s.ready(t)

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "st")
	s.settle(t, "run: st")
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "")

	s.press(t, 'x')
	assertGolden(t, "menu-90x40", s.settle(t, "focus resources panel"))
}

// A key that doesn't act on the row runs whatever a refresh has done to
// the cursor since the palette opened.
func TestThePaletteRunsAPanelKeyAfterTheSelectionMoved(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	selected := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Name
	})

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "filter list instances")
	s.settle(t, "run: filter list instances")
	s.removeInstance(t, selected)
	s.pressKey(t, tcell.KeyEnter)

	assert.NotContains(t, s.settle(t, "filter:"), "isn't selected any more")
	assert.Equal(t, "instances", onLoop(t, s, func() string { return s.gui.State.Filter.panel.GetView().Name() }))
}
