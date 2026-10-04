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

// With no image there's nothing to delete, but pruning still makes sense.
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

	s.typeText(t, "delete")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "incusbr0")
	assert.Equal(t, "confirmation", onLoop(t, s, s.gui.currentViewName))
}
