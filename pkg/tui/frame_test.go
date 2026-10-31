package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

func plainFrame(t *testing.T, border string, s frameSpec, w, h int) []string {
	t.Helper()

	gui := config.GetDefaultConfig().Gui
	gui.Border = border

	return strings.Split(ansi.Strip(newTheme(gui).frame(s, w, h)), "\n")
}

func TestFrameTitle(t *testing.T) {
	lines := plainFrame(t, "", frameSpec{prefix: "[1]", title: "Instances"}, 20, 3)

	assert.Equal(t, []string{
		"╭─[1]─Instances────╮",
		"│                  │",
		"╰──────────────────╯",
	}, lines)
}

func TestFrameTitleKeepsARuleBeforeTheCorner(t *testing.T) {
	lines := plainFrame(t, "", frameSpec{prefix: "[3]", tabs: []string{"Images", "Volumes"}}, 16, 2)

	assert.Equal(t, "╭─[3]─Images -─╮", lines[0])
}

func TestFrameBorders(t *testing.T) {
	for border, want := range map[string]string{
		"single": "┌─x─┐", "double": "╔═x═╗", "hidden": "  x  ", "rounded": "╭─x─╮",
	} {
		assert.Equal(t, want, plainFrame(t, border, frameSpec{title: "x"}, 5, 2)[0], border)
	}
}

func TestHighlightKeepsColoursAndBoldUnderTheBackground(t *testing.T) {
	line := highlight("\x1b[32mrun\x1b[0m ok", 8, "\x1b[44m")

	assert.Equal(t, "\x1b[44m\x1b[1m\x1b[92mrun\x1b[0m\x1b[44m\x1b[1m ok  \x1b[0m\x1b[44m\x1b[1m\x1b[0m", line)
	assert.Equal(t, 8, ansi.StringWidth(line))
}

func TestBrightenLeavesOtherColoursAlone(t *testing.T) {
	assert.Equal(t, "\x1b[1;92;44mx\x1b[0m\x1b[96my\x1b[38;5;33;95mz", brighten("\x1b[1;32;44mx\x1b[0m\x1b[36my\x1b[38;5;33;35mz"))
	assert.Equal(t, "\x1b[38;2;30;31;32mx\x1b[91m", brighten("\x1b[38;2;30;31;32mx\x1b[31m"))
}

func TestFrameSubtitle(t *testing.T) {
	th := newTheme(config.GetDefaultConfig().Gui)

	top := ansi.Strip(strings.Split(th.frame(frameSpec{title: "Commands", subtitle: "esc: close"}, 30, 3), "\n")[0])
	assert.Equal(t, "╭─Commands────esc: close─────╮", top)

	tooWide := ansi.Strip(strings.Split(th.frame(frameSpec{title: "T", subtitle: strings.Repeat("x", 30)}, 30, 3), "\n")[0])
	assert.Equal(t, "╭─T──────────────────────────╮", tooWide)
}
