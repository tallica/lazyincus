package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/config"
)

// The parity tests lay pkg/tui out as a pkg/gui screen test does and
// compare where every frame's corners fall, and the footer, with that
// test's golden screen. What's inside the panels differs until each is
// ported.
type parityCase struct {
	golden        string
	width, height int
	opts          options
	keys          []string
	open          func(*Model)
	footer        bool
}

var parityCases = []parityCase{
	{golden: "normal-140x40", width: 140, height: 40, footer: true},
	{golden: "half-140x40", width: 140, height: 40, keys: []string{"+"}, footer: true},
	{golden: "full-140x40", width: 140, height: 40, keys: []string{"+", "+"}, footer: true},
	{golden: "portrait-84x50", width: 84, height: 50},
	{golden: "expanded-140x40", width: 140, height: 40, keys: []string{"="}, footer: true},
	{golden: "stacks-elsewhere-140x40", width: 140, height: 40, opts: options{compose: true}},
	{golden: "stacks-elsewhere-focused-140x40", width: 140, height: 40, opts: options{compose: true}, keys: []string{"1"}},
	{
		golden: "confirmation-140x40", width: 140, height: 40,
		open: func(m *Model) { m.openConfirmation("Confirm", "Are you sure you want to stop web?", nil) },
	},
	{
		golden: "confirmation-wrapped-140x40", width: 140, height: 40,
		open: func(m *Model) {
			words := strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " " + strings.Repeat("c", 40)
			m.openConfirmation("Confirm", words, nil)
		},
	},
	{golden: "error-140x40", width: 140, height: 40, open: func(m *Model) { m.openError("instance is running") }},
}

func TestLayoutMatchesGocui(t *testing.T) {
	cases := slices.Clone(parityCases)
	for _, border := range []string{"rounded", "single", "double"} {
		cases = append(cases, parityCase{
			golden: "border-" + border + "-100x30", width: 100, height: 30,
			opts: options{configure: func(c *config.UserConfig) { c.Gui.Border = border }},
		})
	}

	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "gui", "testdata", "screens", tc.golden+".txt"))
			require.NoError(t, err)
			want := strings.Split(string(raw), "\n")

			tc.opts.fixture = true
			m := start(t, tc.opts, tc.width, tc.height)
			press(m, tc.keys...)
			if tc.open != nil {
				tc.open(m)
			}
			got := screen(m)

			require.NotEmpty(t, corners(want))
			assert.Equal(t, corners(want), corners(got))
			if d, ok := m.dims()["instances"]; ok && !tc.opts.compose {
				assert.Equal(t, inside(want, d), inside(got, d), "the instances list")
			}
			if tc.footer {
				assert.Equal(t, strings.TrimRight(want[tc.height-1], " "), strings.TrimRight(got[tc.height-1], " "))
			}
		})
	}
}

// inside is the text within d's frame, right-trimmed.
func inside(lines []string, d boxlayout.Dimensions) []string {
	rows := []string{}
	for y := d.Y0 + 1; y < d.Y1 && y < len(lines); y++ {
		row := ansi.Cut(lines[y], d.X0+1, d.X1)
		rows = append(rows, strings.TrimRight(row, " "))
	}

	return rows
}

// corners are where every frame corner is on a screen, as "x,y rune".
func corners(lines []string) []string {
	found := []string{}
	for y, line := range lines {
		x := 0
		for _, r := range line {
			if strings.ContainsRune("╭╮╰╯┌┐└┘╔╗╚╝", r) {
				found = append(found, fmt.Sprintf("%d,%d %c", x, y, r))
			}
			x += ansi.StringWidth(string(r))
		}
	}
	slices.Sort(found)

	return found
}
