package gui

import (
	"regexp"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
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
