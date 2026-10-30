package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
)

// inside is the text within d's frame, right-trimmed.
func inside(lines []string, d boxlayout.Dimensions) []string {
	rows := []string{}
	for y := d.Y0 + 1; y < d.Y1 && y < len(lines); y++ {
		row := ansi.Cut(lines[y], d.X0+1, d.X1)
		rows = append(rows, strings.TrimRight(row, " "))
	}

	return rows
}
