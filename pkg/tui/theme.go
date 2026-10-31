package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// colourCodes are the config's colour names as SGR foregrounds; +10 is the bg.
var colourCodes = map[string]int{
	"black": 30, "red": 31, "green": 32, "yellow": 33,
	"blue": 34, "magenta": 35, "cyan": 36, "white": 37,
	"brightblack": 90, "brightred": 91, "brightgreen": 92, "brightyellow": 93,
	"brightblue": 94, "brightmagenta": 95, "brightcyan": 96, "brightwhite": 97,
}

var attributeCodes = map[string]int{"bold": 1, "underline": 4, "reverse": 7}

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
)

// sgr is the escape for what keys name, "" for nothing or "default".
func sgr(keys []string, background bool) string {
	codes := []string{}

	for _, key := range keys {
		if code, ok := attributeCodes[key]; ok {
			codes = append(codes, strconv.Itoa(code))
			continue
		}

		if utils.IsValidHexValue(key) {
			r, g, b := hexRGB(key)
			lead := "38"
			if background {
				lead = "48"
			}
			codes = append(codes, fmt.Sprintf("%s;2;%d;%d;%d", lead, r, g, b))
			continue
		}

		if code, ok := colourCodes[key]; ok {
			if background {
				code += 10
			}
			codes = append(codes, strconv.Itoa(code))
		}
	}

	if len(codes) == 0 {
		return ""
	}

	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func hexRGB(hex string) (int, int, int) {
	digits := hex[1:]
	if len(digits) == 3 {
		digits = strings.Repeat(digits[0:1], 2) + strings.Repeat(digits[1:2], 2) + strings.Repeat(digits[2:3], 2)
	}

	value, _ := strconv.ParseUint(digits, 16, 32)

	return int(value >> 16 & 0xff), int(value >> 8 & 0xff), int(value & 0xff)
}

// theme is the config's colours as escapes, worked out once per config.
type theme struct {
	activeBorder, inactiveBorder string
	// activeTab is the tab on show in an unfocused window: unbolded.
	activeTab        string
	selectedBg       string
	inactiveSelected string
	options          string
	runes            frameRunes
}

func newTheme(gui config.GuiConfig) theme {
	t := gui.Theme
	unbold := []string{}
	for _, key := range t.ActiveBorderColor {
		if key != "bold" {
			unbold = append(unbold, key)
		}
	}

	return theme{
		activeBorder:     sgr(t.ActiveBorderColor, false),
		inactiveBorder:   sgr(t.InactiveBorderColor, false),
		activeTab:        sgr(unbold, false),
		selectedBg:       sgr(t.SelectedLineBgColor, true),
		inactiveSelected: sgr(t.InactiveSelectedLineBgColor, true),
		options:          sgr(t.OptionsTextColor, false),
		runes:            borderRunes(gui.Border),
	}
}

// applyTheme: with no colour for a selection, gocui's tcell fell back to reverse.
func (m *Model) applyTheme() {
	m.theme = newTheme(m.cfg.UserConfig.Gui)
	if m.monochrome {
		m.theme.selectedBg, m.theme.inactiveSelected = "\x1b[7m", "\x1b[7m"
	}
}

// frameRunes: horizontal, vertical, then corners clockwise from top left.
type frameRunes struct {
	h, v, tl, tr, br, bl string
}

func borderRunes(border string) frameRunes {
	switch border {
	case "single":
		return frameRunes{"─", "│", "┌", "┐", "┘", "└"}
	case "double":
		return frameRunes{"═", "║", "╔", "╗", "╝", "╚"}
	case "hidden":
		return frameRunes{" ", " ", " ", " ", " ", " "}
	}

	return frameRunes{"─", "│", "╭", "╮", "╯", "╰"}
}

// remoteColour is a remote's configured colour as an escape, "" for none.
func remoteColour(cfg *config.UserConfig, remote string) string {
	key := cfg.Remotes[remote].Color
	if key == "" {
		return ""
	}

	return sgr([]string{key}, false)
}
