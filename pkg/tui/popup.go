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

// popup is a message, or with asks a question; the top one has the keys.
type popup struct {
	title    string
	message  string
	asks     bool
	bindings []*binding
}

func (m *Model) openError(message string) {
	m.openMessage(m.tr.ErrorTitle, utils.ColoredString(strings.TrimSpace(message), color.FgRed))
}

// openMessage is pkg/gui's createMessagePanel.
func (m *Model) openMessage(title, message string) {
	m.openPopup(&popup{title: title, message: message}, nil, nil)
}

// openConfirmation is pkg/gui's createConfirmationPanel.
func (m *Model) openConfirmation(title, message string, onYes func() tea.Cmd) {
	m.openPopup(&popup{title: title, message: message, asks: true}, onYes, nil)
}

// openPopup is pkg/gui's createPopupPanel: enter/y confirm, esc/n close.
func (m *Model) openPopup(p *popup, onConfirm, onClose func() tea.Cmd) {
	closing := func(f func() tea.Cmd) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.closePopup()
			if f == nil {
				return nil
			}
			return f()
		}
	}

	confirm, closeIt := closing(onConfirm), closing(onClose)
	for _, k := range []struct {
		key     string
		handler func(*Model) tea.Cmd
	}{{"enter", confirm}, {"y", confirm}, {"esc", closeIt}, {"n", closeIt}} {
		p.bindings = append(p.bindings, &binding{view: popupView, key: k.key, handler: k.handler})
	}

	m.popups = append(m.popups, p)
}

func (m *Model) closePopup() {
	m.popups = m.popups[:len(m.popups)-1]
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
