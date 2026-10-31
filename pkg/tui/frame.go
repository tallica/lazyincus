package tui

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// frameSpec is what a framed window shows; frame draws it at a size.
type frameSpec struct {
	prefix string
	title  string
	// subtitle sits on the top edge five in from the corner, footer on the bottom one in.
	subtitle string
	footer   string
	tabs     []string
	tab      int
	body     []string
	focused  bool
	// colour is the frame's escape while unfocused, "" for the theme's.
	colour string
	// total and origin place the scrollbar: content lines, first one shown.
	total, origin int
}

// frame draws s in a w×h box, its title in the top edge: lipgloss borders
// take no title.
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
	top := border + r.tl + r.h + reset + heading + border + rule + r.tr + reset
	// A subtitle that would start left of the frame is skipped.
	if sw := ansi.StringWidth(s.subtitle); s.subtitle != "" && w-6-sw >= 0 {
		top = ansi.Truncate(top, w-6-sw, "") + reset + border + s.subtitle + strings.Repeat(r.h, 5) + r.tr + reset
	}
	lines = append(lines, top)

	side := border + r.v + reset
	start, end := scrollbar(s.total, h-2, s.origin)
	for i := range h - 2 {
		line := strings.Repeat(" ", inner)
		if i < len(s.body) {
			line = fit(s.body[i], inner)
		}
		right := side
		if i >= start && i <= end {
			right = border + "▐" + reset
		}
		lines = append(lines, side+line+right)
	}

	bottom := border + r.bl + strings.Repeat(r.h, inner) + r.br + reset
	// A footer that would start left of the frame is skipped.
	if fw := ansi.StringWidth(s.footer); s.footer != "" && w-2-fw >= 1 {
		bottom = border + r.bl + strings.Repeat(r.h, w-3-fw) + s.footer + r.h + r.br + reset
	}
	lines = append(lines, bottom)

	return strings.Join(lines, "\n")
}

// heading colours tabs: focused, only the tab on show is
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

// scrollbar is the rows the thumb covers,
// end inclusive; start > end for none. Adapted from gocui's calcScrollbar;
// see THIRD_PARTY_NOTICES.md.
func scrollbar(total, height, origin int) (int, int) {
	if height < 2 || height >= total {
		return 0, -1
	}

	area := height - 1
	size := int(float64(height) / float64(total) * float64(area))

	start := 0
	if maxOrigin := total - height; origin == maxOrigin {
		start = area - size
	} else if maxOrigin > 0 {
		start = int(math.Ceil(float64(origin) / float64(maxOrigin) * float64(area-size-1)))
	}

	return start, start + size
}

// fit cuts or pads line to exactly width cells, colour codes kept.
func fit(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}

	return line + reset
}

var sgrSequence = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// brighten turns line's 8-colour foregrounds bright, as on a selected row.
func brighten(line string) string {
	return sgrSequence.ReplaceAllStringFunc(line, func(seq string) string {
		params := strings.Split(sgrSequence.FindStringSubmatch(seq)[1], ";")
		for i := 0; i < len(params); i++ {
			code, err := strconv.Atoi(params[i])
			switch {
			case err != nil:
			case code == 38 || code == 48:
				i += 2
				if i-1 < len(params) && params[i-1] == "2" {
					i += 2
				}
			case code >= 30 && code <= 37:
				params[i] = strconv.Itoa(code + 60)
			}
		}

		return "\x1b[" + strings.Join(params, ";") + "m"
	})
}

// highlight puts bg behind line, bold, re-asserting both after each reset inside.
func highlight(line string, width int, bg string) string {
	bg += bold
	line = strings.ReplaceAll(brighten(fit(line, width)), reset, reset+bg)

	return bg + line + reset
}
