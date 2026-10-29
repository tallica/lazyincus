package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The cases are gocui's own TestLineWrap.
func TestWrapLineAsGocui(t *testing.T) {
	for _, tc := range []struct {
		line    string
		columns int
		want    []string
	}{
		{"Hello World", 5, []string{"Hello", "World"}},
		{"Hello-World", 6, []string{"Hello-", "World"}},
		{"Blah Hello-World", 12, []string{"Blah Hello-", "World"}},
		{"Blah Hello-World", 11, []string{"Blah Hello-", "World"}},
		{"Blah Hello-World", 10, []string{"Blah Hello", "-World"}},
		{"Blah Hello World", 10, []string{"Blah Hello", "World"}},
		{"Longer word here", 10, []string{"Longer", "word here"}},
		{"ThisWordIsWayTooLong", 10, []string{"ThisWordIs", "WayTooLong"}},
		{"ThisWordIsWayTooLong", 5, []string{"ThisW", "ordIs", "WayTo", "oLong"}},
		{"one-two-three-four-five", 8, []string{"one-two-", "three-", "four-", "five"}},
		{"aaa bb cc ddd-ee ff", 5, []string{"aaa", "bb cc", "ddd-", "ee ff"}},
		{"🐤🐤🐤 🐝🐝 🙉 🦊🦊🦊-🐬🐬 🦢🦢", 9, []string{"🐤🐤🐤", "🐝🐝 🙉", "🦊🦊🦊-", "🐬🐬 🦢🦢"}},
	} {
		assert.Equal(t, tc.want, wrapLine(tc.line, tc.columns), "%q at %d", tc.line, tc.columns)
	}
}

// border-rounded-100x30 in pkg/gui's goldens: 28 lines in 27 rows.
func TestScrollbarAsGocui(t *testing.T) {
	start, end := scrollbar(28, 27, 0)
	assert.Equal(t, []int{0, 25}, []int{start, end})

	start, end = scrollbar(28, 27, 1)
	assert.Equal(t, []int{1, 26}, []int{start, end}, "snaps to the bottom at the end")

	start, end = scrollbar(27, 27, 0)
	assert.Greater(t, start, end, "none when it all fits")
}

// setPane is a task's first write to a pane sized width by height.
func setPane(p *pane, content string, wrap, autoscroll bool, width, height int) {
	p.fit(width, height)
	p.begin(wrap, autoscroll)
	p.render(content)
}

func numbered(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = strconv.Itoa(i)
	}

	return strings.Join(lines, "\n")
}

func TestPaneAutoscrollKeepsTrailingBlankLinesInView(t *testing.T) {
	var p pane
	setPane(&p, numbered(10)+"\n\n\n", false, true, 20, 4)

	assert.Equal(t, []string{"8", "9", "", ""}, p.visible(4))
}

func TestPaneScrollingStopsAtTheEnds(t *testing.T) {
	var p pane
	setPane(&p, numbered(10), false, false, 20, 4)

	p.scrollDown(4, 4, false)
	p.scrollDown(4, 4, false)
	assert.Equal(t, 8, p.oy)
	p.scrollDown(4, 4, false)
	assert.Equal(t, 8, p.oy, "the last line is already in view")
	assert.Equal(t, []string{"8", "9"}, p.visible(4))

	p.scrollUp(5)
	p.scrollUp(5)
	assert.Equal(t, 0, p.oy)
}

func TestPaneScrollsSidewaysToTheWidestLine(t *testing.T) {
	var p pane
	setPane(&p, "short\n"+strings.Repeat("x", 12)+"y", false, false, 8, 4)

	p.scrollRight(4)
	p.scrollRight(4)
	assert.Equal(t, 8, p.ox)
	p.scrollRight(4)
	assert.Equal(t, 8, p.ox, "the widest line already ends inside")
	assert.Equal(t, []string{"", "xxxxy"}, p.visible(4))

	p.scrollLeft(10)
	assert.Equal(t, 0, p.ox)
}

func TestWrappedPaneNeverScrollsSideways(t *testing.T) {
	var p pane
	setPane(&p, "abcdefghij", true, false, 4, 4)
	p.ox = 4

	assert.Equal(t, []string{"abcd", "efgh", "ij"}, p.visible(4))
}

func TestPaneKeepsItsPlaceUntilRenderedAfresh(t *testing.T) {
	var p pane
	setPane(&p, numbered(10), false, false, 20, 4)
	p.scrollDown(3, 4, false)

	p.rerender(numbered(12))
	assert.Equal(t, 3, p.oy, "a ticker redrawing the same thing")

	p.begin(false, true)
	p.render(numbered(12))
	assert.Equal(t, 8, p.oy, "a new task starts afresh, its own autoscroll on")
}

func TestPaneContentAsGocuiWritesIt(t *testing.T) {
	assert.Equal(t, []string{"a   b", "ab  c", ""}, viewLines("a\tb\r\nab\tc\n\n", false, 20))
	assert.Equal(t, []string{""}, viewLines("", false, 20))
	assert.Equal(t, []string{""}, viewLines("\n", false, 20))
}

func TestPaneShowsTheLastLineWhenScrolledPastIt(t *testing.T) {
	var p pane
	setPane(&p, numbered(3), false, false, 20, 4)
	p.oy = 7

	assert.Equal(t, []string{"2"}, p.visible(4))
}
