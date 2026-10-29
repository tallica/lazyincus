package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Match colours are gocui's, fixed rather than themed.
const (
	matchStyle        = "\x1b[30;43m"
	currentMatchStyle = "\x1b[30;46m"
)

// match is a needle found on view line y, over columns [x0, x1).
type match struct {
	x0, x1, y int
}

// search is gocui's searcher: the needle, its matches, the current one.
type search struct {
	needle  string
	matches []match
	current int
}

func (p *pane) searching() bool {
	return p.search.needle != ""
}

// find is gocui's View.Search: the main view never highlights a line, so
// it starts at the first match.
func (p *pane) find(needle string) {
	p.search = search{needle: needle}
	p.findMatches()
	if len(p.search.matches) > 0 {
		p.selectMatch(0)
	}
}

func (p *pane) clearSearch() {
	p.search = search{}
}

// findMatches is gocui's updateSearchPositions: case-sensitive only when
// the needle has a capital, and never across a wrapped line's break.
func (p *pane) findMatches() {
	p.search.matches = nil
	if !p.searching() {
		return
	}

	needle := p.search.needle
	fold := !strings.ContainsFunc(needle, unicode.IsUpper)
	if fold {
		needle = strings.ToLower(needle)
	}
	want := cells(needle)
	width := ansi.StringWidth(p.search.needle)

	for y, line := range p.lines {
		cs := cells(line)
		x := 0
		for i := range cs {
			if matchesAt(cs[i:], want, fold) {
				p.search.matches = append(p.search.matches, match{x0: x, x1: x + width, y: y})
			}
			x += cs[i].width
		}
	}
}

func matchesAt(cs, want []cell, fold bool) bool {
	if len(cs) < len(want) {
		return false
	}

	for i, w := range want {
		chr := cs[i].chr
		if fold {
			chr = strings.ToLower(chr)
		}
		if chr != w.chr {
			return false
		}
	}

	return true
}

// refind is gocui's on each write: the current match moves if it's gone.
func (p *pane) refind() {
	p.findMatches()
	if n := len(p.search.matches); n > 0 && p.search.current >= n {
		p.search.current = p.nearestMatch()
	}
}

// nearestMatch is gocui's nearestSearchPosition, from the cursor's line.
func (p *pane) nearestMatch() int {
	line := p.cy + p.oy
	nearest := 0
	for i, m := range p.search.matches {
		if m.y == line {
			return i
		}
		if m.y > line {
			break
		}
		nearest = i
	}

	return nearest
}

// nextMatch is gocui's gotoNextMatch (1) and gotoPreviousMatch (-1).
func (p *pane) nextMatch(step int) {
	n := len(p.search.matches)
	if n == 0 {
		return
	}

	current := min(p.search.current, n-1)
	switch {
	case step > 0 && current >= n-1:
		current = 0
	case step < 0 && current == 0:
		current = n - 1
	default:
		current += step
	}

	p.selectMatch(current)
}

func (p *pane) selectMatch(i int) {
	p.search.current = i
	y := p.search.matches[i].y
	if y > len(p.lines) {
		return
	}

	p.oy = originFor(y, p.oy, len(p.lines), p.height)
	p.cy = y - p.oy
}

// originFor is gocui's calculateNewOrigin: a line out of view goes mid-way.
func originFor(line, origin, lines, height int) int {
	if height >= lines {
		return 0
	}
	if line >= origin && line < origin+height {
		return origin
	}

	return max(min(line-height/2, lines-height), 0)
}

func (p *pane) matchStatus() (int, int) {
	return p.search.current, len(p.search.matches)
}

// highlightMatches colours line y's matches as gocui's setCharacter does:
// black on yellow, the current one on cyan, whatever the text's own style.
func (p *pane) highlightMatches(line string, y int) string {
	matchAt := func(x int) int {
		for i, m := range p.search.matches {
			if m.y == y && x >= m.x0 && x < m.x1 {
				return i
			}
		}

		return -1
	}

	var b strings.Builder
	active := ""
	x := 0
	for _, c := range cells(line) {
		b.WriteString(c.escapes)
		active = carry(active, c.escapes)

		i := matchAt(x)
		switch {
		case c.chr == "" || i < 0:
			b.WriteString(c.chr)
		case i == p.search.current:
			b.WriteString(reset + currentMatchStyle + c.chr + reset + active)
		default:
			b.WriteString(reset + matchStyle + c.chr + reset + active)
		}
		x += c.width
	}

	return b.String()
}

func (p *pane) hasMatchOn(y int) bool {
	for _, m := range p.search.matches {
		if m.y == y {
			return true
		}
	}

	return false
}
