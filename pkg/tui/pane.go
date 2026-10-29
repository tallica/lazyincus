package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/spkg/bom"
	"github.com/tallica/lazyincus/pkg/utils"
)

// tabWidth is gocui's default View.TabWidth, which pkg/gui leaves alone.
const tabWidth = 4

// cell is one grapheme of a styled line, with the escapes written before it.
type cell struct {
	escapes string
	chr     string
	width   int
}

func cells(line string) []cell {
	var (
		out     []cell
		escapes string
		state   byte
	)

	for len(line) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(line, state, nil)
		state = newState
		line = line[n:]

		if width == 0 && strings.HasPrefix(seq, "\x1b") {
			escapes += seq
			continue
		}

		out = append(out, cell{escapes: escapes, chr: seq, width: width})
		escapes = ""
	}

	if escapes != "" {
		out = append(out, cell{escapes: escapes})
	}

	return out
}

// wrapLine breaks line to columns cells as gocui's lineWrap does: at the
// last space or hyphen that fits, dropping a space and keeping a hyphen,
// else mid-word. A continuation starts with the escapes still in force,
// as gocui's cells each carry their own style.
func wrapLine(line string, columns int) []string {
	if columns <= 0 {
		return []string{line}
	}

	cs := cells(line)
	var pieces [][]cell
	n, offset, lastSpace := 0, 0, -1

	for i, c := range cs {
		n += c.width
		if n > columns {
			switch {
			case c.chr == " ":
				pieces = append(pieces, cs[offset:i])
				offset, n = i+1, 0
			case c.chr == "-":
				pieces = append(pieces, cs[offset:i])
				offset, n = i, c.width
			case lastSpace != -1:
				end := lastSpace
				if cs[lastSpace].chr == "-" {
					end++
				}
				pieces = append(pieces, cs[offset:end])
				offset, n = lastSpace+1, 0
				for _, rest := range cs[offset : i+1] {
					n += rest.width
				}
			default:
				pieces = append(pieces, cs[offset:i])
				offset, n = i, c.width
			}
			lastSpace = -1
		} else if c.chr == " " || c.chr == "-" {
			lastSpace = i
		}
	}
	pieces = append(pieces, cs[offset:])

	lines := make([]string, len(pieces))
	active := ""
	for i, piece := range pieces {
		var b strings.Builder
		b.WriteString(active)
		for _, c := range piece {
			b.WriteString(c.escapes)
			b.WriteString(c.chr)
			active = carry(active, c.escapes)
		}
		lines[i] = b.String()
	}

	return lines
}

// carry is the escapes in force after escapes: those since the last reset.
func carry(active, escapes string) string {
	active += escapes
	for _, r := range []string{reset, "\x1b[m"} {
		if i := strings.LastIndex(active, r); i >= 0 {
			active = active[i+len(r):]
		}
	}

	return active
}

// pane is the main panel's content as gocui's main view holds it.
type pane struct {
	// key is what the content belongs to; a new one starts afresh.
	key        string
	content    string
	wrap       bool
	autoscroll bool

	lines  []string
	width  int
	height int
	ox     int
	oy     int
	// cy is gocui's cursor row, relative to oy: where a search last
	// focused, which a scroll doesn't move.
	cy int

	search search
}

// set takes new content, re-wrapping only when it or the width changed.
func (p *pane) set(key, content string, wrap, autoscroll bool, width, height int) {
	if key != p.key {
		// gocui's Clear leaves the view's search be.
		*p = pane{key: key, autoscroll: autoscroll, search: p.search}
	}
	p.height = height

	if content != p.content || wrap != p.wrap || width != p.width || p.lines == nil {
		p.content, p.wrap, p.width = content, wrap, width
		p.lines = viewLines(content, wrap, width)
		p.refind()
	}

	// gocui's autoscroll skips trailing lines with no cells, but writing
	// gives every line at least one.
	if p.autoscroll && len(p.lines) > height {
		p.oy = len(p.lines) - height
	}
}

// viewLines is content as gocui's main view holds it once pkg/gui's
// setViewContent has written it: a final newline starts no line, but
// nothing at all is still one.
func viewLines(content string, wrap bool, width int) []string {
	content = utils.NormalizeLinefeeds(string(bom.Clean([]byte(content))))
	content = strings.TrimSuffix(content, "\n")

	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = expandTabs(line)
		if wrap {
			lines = append(lines, wrapLine(line, width)...)
		} else {
			lines = append(lines, line)
		}
	}

	return lines
}

// expandTabs pads each tab to the next stop, counted in cells as gocui's
// parseInput does, not columns.
func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}

	var b strings.Builder
	x := 0
	for _, c := range cells(line) {
		b.WriteString(c.escapes)
		if c.chr == "\t" {
			n := tabWidth - x%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			x += n
			continue
		}
		b.WriteString(c.chr)
		if c.chr != "" {
			x++
		}
	}

	return b.String()
}

// visible is what a height-row window onto the pane shows: from the last
// line if scrolled past it, and never sideways when wrapped, as gocui draws.
func (p *pane) visible(height int) []string {
	start := max(min(p.oy, len(p.lines)-1), 0)
	end := min(start+height, len(p.lines))

	rows := make([]string, 0, max(end-start, 0))
	for y, line := range p.lines[start:end] {
		if p.hasMatchOn(start + y) {
			line = p.highlightMatches(line, start+y)
		}
		if p.ox > 0 && !p.wrap {
			line = ansi.Cut(line, p.ox, p.ox+p.width)
		}
		rows = append(rows, line)
	}

	return rows
}

// scrollDown is pkg/gui's scrollDownMain.
func (p *pane) scrollDown(step, height int, pastBottom bool) {
	p.autoscroll = false

	reserved := height
	if pastBottom {
		reserved = 0
	}
	if p.oy+reserved >= len(p.lines) {
		return
	}

	p.oy += step
}

func (p *pane) scrollUp(step int) {
	p.autoscroll = false
	p.oy = max(p.oy-step, 0)
}

// scrollRight is pkg/gui's scrollRightMain.
func (p *pane) scrollRight(step int) {
	widest := 0
	for _, line := range p.lines {
		widest = max(widest, ansi.StringWidth(line))
	}

	if p.ox+p.width >= widest {
		return
	}

	p.ox += step
}

func (p *pane) scrollLeft(step int) {
	p.ox = max(p.ox-step, 0)
}
