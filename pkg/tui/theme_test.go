package tui

import (
	"image/color"
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestAMonochromeTerminalShowsTheSelectionInReverseVideo(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.cfg.UserConfig.Gui.Theme.SelectedLineBgColor = []string{"blue"}
	m.applyTheme()
	reverse := regexp.MustCompile(`\x1b\[(\d+;)*7(;\d+)*m`)
	row := func() string { return m.render() }

	assert.False(t, reverse.MatchString(row()))

	m.Update(tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	assert.True(t, reverse.MatchString(row()))

	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	assert.False(t, reverse.MatchString(row()))
}

func TestA16ColourTermCapsTheDetectedProfile(t *testing.T) {
	for _, tc := range []struct {
		term     string
		detected colorprofile.Profile
		want     colorprofile.Profile
	}{
		{"xterm-16color", colorprofile.ANSI256, colorprofile.ANSI},
		{"xterm-16color", colorprofile.TrueColor, colorprofile.ANSI},
		{"xterm-color", colorprofile.ANSI256, colorprofile.ANSI},
		{"xterm-16color", colorprofile.ASCII, colorprofile.ASCII},
		{"xterm-256color", colorprofile.TrueColor, colorprofile.TrueColor},
		{"", colorprofile.ANSI256, colorprofile.ANSI256},
	} {
		assert.Equal(t, tc.want, terminalProfile(tc.detected, tc.term), tc.term)
	}
}

// A light background takes the light defaults for the keys the config leaves unset.
func TestALightBackgroundTakesTheLightDefaults(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) {
		c.Gui.Theme.InactiveSelectedLineBgColor = []string{"#112233"}
	}}, 140, 40)
	assert.Equal(t, sgr(config.DarkTheme.SelectedLineBgColor, true), m.theme.selectedBg)
	assert.True(t, m.theme.dark)

	m.Update(tea.BackgroundColorMsg{Color: color.White})
	assert.Equal(t, sgr(config.LightTheme.SelectedLineBgColor, true), m.theme.selectedBg)
	assert.Equal(t, sgr([]string{"#112233"}, true), m.theme.inactiveSelected, "a key the config sets keeps it")
	assert.False(t, m.theme.dark)

	m.Update(tea.BackgroundColorMsg{Color: color.Black})
	assert.Equal(t, sgr(config.DarkTheme.SelectedLineBgColor, true), m.theme.selectedBg)
}

// A selected row is brightened on dark only, and bold only where the theme says so.
func TestHighlightBrightensOnDarkAndBoldsAsTheThemeSays(t *testing.T) {
	dark := highlight("\x1b[32mrun\x1b[0m", 3, "", true)
	assert.Contains(t, dark, "\x1b[92m")
	assert.NotContains(t, dark, bold)

	light := highlight("\x1b[32mrun\x1b[0m", 3, "", false)
	assert.Contains(t, light, "\x1b[32m")

	m := start(t, options{configure: func(c *config.UserConfig) {
		c.Gui.Theme.SelectedLineBgColor = []string{"#515151"}
	}}, 140, 40)
	assert.Equal(t, "\x1b[48;2;81;81;81m", m.theme.selectedBg, "no bold unless the key asks")
	assert.Equal(t, "\x1b[48;2;59;59;59;1m", m.theme.inactiveSelected, "the default inactive bar keeps its bold")
}
