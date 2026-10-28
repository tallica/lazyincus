package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// frameSpec is what a framed window shows; frame draws it at a size.
type frameSpec struct {
	prefix  string
	title   string
	tabs    []string
	tab     int
	body    []string
	focused bool
	// colour is the frame's escape while unfocused, "" for the theme's.
	colour string
}

// frame draws s in a w×h box, its title in the top edge where gocui's
// drawTitle puts it: lipgloss borders take no title.
func (t theme) frame(s frameSpec, w, h int) string {
	if w < 2 || h < 2 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", max(w, 0))+"\n", max(h, 0)), "\n")
	}

	border := t.inactiveBorder
	if s.colour != "" {
		border = s.colour
	}
	if s.focused {
		border = t.activeBorder
	}

	r := t.runes
	inner := w - 2

	heading := ansi.Truncate(t.heading(s, border), max(w-4, 0), "")
	rule := strings.Repeat(r.h, max(w-3-ansi.StringWidth(heading), 0))

	lines := make([]string, 0, h)
	lines = append(lines, border+r.tl+r.h+reset+heading+border+rule+r.tr+reset)

	side := border + r.v + reset
	for i := range h - 2 {
		line := strings.Repeat(" ", inner)
		if i < len(s.body) {
			line = fit(s.body[i], inner)
		}
		lines = append(lines, side+line+side)
	}

	lines = append(lines, border+r.bl+strings.Repeat(r.h, inner)+r.br+reset)

	return strings.Join(lines, "\n")
}

// heading colours tabs as gocui does: focused, only the tab on show is
// coloured; unfocused, everything is, that tab unbolded.
func (t theme) heading(s frameSpec, border string) string {
	prefix := ""
	if s.prefix != "" {
		prefix = border + s.prefix + t.runes.h + reset
	}

	if len(s.tabs) == 0 {
		return prefix + border + s.title + reset
	}

	other := border
	if s.focused {
		other = ""
	}

	parts := make([]string, len(s.tabs))
	for i, tab := range s.tabs {
		switch {
		case i == s.tab && s.focused:
			parts[i] = t.activeBorder + tab + reset
		case i == s.tab:
			parts[i] = t.activeTab + tab + reset
		default:
			parts[i] = other + tab + reset
		}
	}

	return prefix + strings.Join(parts, other+" - "+reset)
}

// fit cuts or pads line to exactly width cells, colour codes kept.
func fit(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}

	return line + reset
}

// highlight puts bg behind line, re-asserting it after each reset inside.
func highlight(line string, width int, bg string) string {
	if bg == "" {
		return fit(line, width)
	}

	line = strings.ReplaceAll(fit(line, width), reset, reset+bg)

	return bg + line + reset
}
