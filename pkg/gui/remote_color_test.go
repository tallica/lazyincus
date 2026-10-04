package gui

import (
	"slices"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// A stack on a coloured remote colours Stacks and Services; the session's
// panels keep the session remote's colour.
func TestFramesTakeTheirRemotesColor(t *testing.T) {
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"pve01": {Color: "red"}, "fake": {Color: "#00ff00"}}
	}, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
	})

	landed(t, s, "instances")
	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"shop/api"})
	}, 5*time.Second, 20*time.Millisecond)
	s.ready(t)

	frames := onLoop(t, s, func() map[string]gocui.Attribute {
		gui := s.gui
		return map[string]gocui.Attribute{
			"stacks":    gui.Views.Stacks.FrameColor,
			"services":  gui.Views.Services.FrameColor,
			"instances": gui.Views.Instances.FrameColor,
			"main":      gui.Views.Main.FrameColor,
		}
	})
	assert.Equal(t, gocui.ColorRed, frames["stacks"])
	assert.Equal(t, gocui.ColorRed, frames["services"])
	assert.Equal(t, gocui.NewRGBColor(0, 0xff, 0), frames["instances"])
	assert.Equal(t, gocui.NewRGBColor(0, 0xff, 0), frames["main"])
}

func TestAnUncolouredRemoteKeepsTheThemesBorder(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	assert.Equal(t, gocui.ColorDefault, onLoop(t, s, func() gocui.Attribute { return s.gui.Views.Instances.FrameColor }))
}

func TestTheFooterColorsTheRemoteName(t *testing.T) {
	noColor := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = noColor })

	gui := bareGui(t)
	gui.Config = &config.AppConfig{UserConfig: &config.UserConfig{Remotes: map[string]config.RemoteConfig{
		"lenny": {Color: "red"}, "site-a": {Color: "#f08"},
	}}}

	assert.Equal(t, "\x1b[31mlenny\x1b[0m", gui.remoteName("lenny"))
	assert.Equal(t, "\x1b[38;2;255;0;136msite-a\x1b[0m", gui.remoteName("site-a"))
	assert.Equal(t, "local", gui.remoteName("local"))
}
