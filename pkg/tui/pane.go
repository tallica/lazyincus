package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/spkg/bom"
	"github.com/tallica/lazyincus/pkg/utils"
)

// tabWidth is the width of a tab stop.
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

// wrapLine breaks line to columns cells: at the
// last space or hyphen that fits, dropping a space and keeping a hyphen,
// else mid-word. A continuation starts with the escapes still in force,
// so each row carries its own style.
func wrapLine(line string, columns int) []string {
	if columns <= 0 {
		return []string{line}
	}

	cs := cells(line)
	var pieces [][]cell
	for _, r := range wrapRanges(cs, columns) {
		pieces = append(pieces, cs[r[0]:r[1]])
	}

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

// wrapRanges are where wrapLine breaks cs, as [start, end) cell indices.
func wrapRanges(cs []cell, columns int) [][2]int {
	var ranges [][2]int
	n, offset, lastSpace := 0, 0, -1

	for i, c := range cs {
		n += c.width
		if n > columns {
			switch {
			case c.chr == " ":
				ranges = append(ranges, [2]int{offset, i})
				offset, n = i+1, 0
			case c.chr == "-":
				ranges = append(ranges, [2]int{offset, i})
				offset, n = i, c.width
			case lastSpace != -1:
				end := lastSpace
				if cs[lastSpace].chr == "-" {
					end++
				}
				ranges = append(ranges, [2]int{offset, end})
				offset, n = lastSpace+1, 0
				for _, rest := range cs[offset : i+1] {
					n += rest.width
				}
			default:
				ranges = append(ranges, [2]int{offset, i})
				offset, n = i, c.width
			}
			lastSpace = -1
		} else if c.chr == " " || c.chr == "-" {
			lastSpace = i
		}
	}

	return append(ranges, [2]int{offset, len(cs)})
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

// pane is the main panel's content, its scroll and its search.
type pane struct {
	content    string
	wrap       bool
	autoscroll bool

	lines  []string
	width  int
	height int
	ox     int
	oy     int
	// cy is the cursor row, relative to oy: where a search last
	// focused, which a scroll doesn't move.
	cy int

	search search
}

// fit sizes the pane, re-wrapping only when the width changed.
func (p *pane) fit(width, height int) {
	p.height = height
	if width != p.width {
		p.width = width
		p.relayout()
	}
	p.follow()
}

// begin is a new task's first write setting the view up for it.
func (p *pane) begin(wrap, autoscroll bool) {
	p.wrap, p.autoscroll = wrap, autoscroll
}

// render: the content, from the top.
func (p *pane) render(content string) {
	p.ox, p.oy, p.cy = 0, 0, 0
	p.rerender(content)
}

// rerender: the content, where it's scrolled to.
func (p *pane) rerender(content string) {
	p.content = content
	p.relayout()
	p.follow()
}

func (p *pane) clear() {
	p.content, p.lines = "", nil
	p.ox, p.oy, p.cy = 0, 0, 0
	p.refind()
}

func (p *pane) relayout() {
	p.lines = viewLines(p.content, p.wrap, p.width)
	p.refind()
}

// follow keeps the last line in view while autoscrolling: every line has
// a cell, so none is skipped as trailing.
func (p *pane) follow() {
	if p.autoscroll && len(p.lines) > p.height {
		p.oy = len(p.lines) - p.height
	}
}

// viewLines is content split into view lines: a final newline starts no
// line, but nothing at all is still one.
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

// expandTabs pads each tab to the next stop, counted in cells, not
// columns.
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
// line if scrolled past it, and never sideways when wrapped.
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
