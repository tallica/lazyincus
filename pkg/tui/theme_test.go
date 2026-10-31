package tui

import (
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
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
