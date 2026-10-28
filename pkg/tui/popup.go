package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/utils"
)

// popupView is what currentView names while a popup has the focus.
const popupView = "popup"

// popup is a message, or with onYes a question; the top one has the keys.
type popup struct {
	title   string
	message string
	onYes   func() tea.Cmd
}

func (m *Model) openError(message string) {
	m.popups = append(m.popups, &popup{
		title:   m.tr.ErrorTitle,
		message: utils.ColoredString(strings.TrimSpace(message), color.FgRed),
	})
}

func (m *Model) openConfirmation(title, message string, onYes func() tea.Cmd) {
	m.popups = append(m.popups, &popup{title: title, message: message, onYes: onYes})
}

func (m *Model) closePopup() {
	m.popups = m.popups[:len(m.popups)-1]
}

// popupKey reports whether the top popup took key; gocui passes the rest on.
func (m *Model) popupKey(key string) (tea.Cmd, bool) {
	top := m.popups[len(m.popups)-1]

	switch {
	case top.onYes == nil && (key == "esc" || key == "enter"):
		m.closePopup()
		return nil, true
	case top.onYes != nil && (key == "n" || key == "esc"):
		m.closePopup()
		return nil, true
	case top.onYes != nil && (key == "y" || key == "enter"):
		m.closePopup()
		return top.onYes(), true
	}

	return nil, false
}

// popupColumns and popupRows are pkg/gui's.
func popupColumns(screenWidth, minWidth int) (int, int) {
	x0, x1 := screenWidth/2-screenWidth/4, screenWidth/2+screenWidth/4
	if minWidth <= x1-x0 {
		return x0, x1
	}

	width := min(minWidth, screenWidth-1)
	x0 = (screenWidth - 1 - width) / 2

	return x0, x0 + width
}

func popupRows(screenHeight, rows int) (int, int) {
	rows = max(1, min(rows, screenHeight-4))

	return screenHeight/2 - rows/2 - rows%2 - 1, screenHeight/2 + rows/2
}

// popupLayout is p's rectangle on this screen, and its wrapped lines.
func popupLayout(p *popup, width, height int) (x0, y0, x1, y1 int, lines []string) {
	x0, x1 = popupColumns(width, 0)
	lines = strings.Split(ansi.Wrap(p.message, max(x1-x0-1, 1), ""), "\n")
	y0, y1 = popupRows(height, len(lines))

	return x0, y0, x1, y1, lines
}
