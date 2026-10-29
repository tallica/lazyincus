package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/utils"
)

// popup is a message, or with asks a question; the top one has the keys.
type popup struct {
	// view is pkg/gui's view for it: confirmationView or menuView.
	view     string
	title    string
	message  string
	asks     bool
	bindings []*binding
	menu     *menu
	// subtitle is the hint as written, fittedSubtitle as drawn.
	subtitle       string
	fittedSubtitle string
	heldWidth      int
}

func (m *Model) openError(message string) {
	m.openMessage(m.tr.ErrorTitle, utils.ColoredString(strings.TrimSpace(message), color.FgRed))
}

// openMessage is pkg/gui's createMessagePanel.
func (m *Model) openMessage(title, message string) {
	m.openPopup(&popup{view: confirmationView, title: title, message: message}, nil, nil)
}

// openConfirmation is pkg/gui's createConfirmationPanel.
func (m *Model) openConfirmation(title, message string, onYes func() tea.Cmd) {
	m.openPopup(&popup{view: confirmationView, title: title, message: message, asks: true}, onYes, nil)
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
		p.bindings = append(p.bindings, &binding{view: confirmationView, key: k.key, handler: k.handler})
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

// popupFrameMargin is pkg/gui's: corners and rule around title and subtitle.
const popupFrameMargin = 10

// fitBeside is pkg/gui's: subtitle cut to the room beside title.
func fitBeside(subtitle, title string, width int) string {
	room := width - utils.DisplayWidth(title) - popupFrameMargin
	if utils.DisplayWidth(subtitle) <= room {
		return subtitle
	}

	if room < 4 {
		return ""
	}

	return utils.Truncate(subtitle, room)
}

// popupX is where pkg/gui's resizePopupPanel puts p across the screen.
func (m *Model) popupX(p *popup) (int, int) {
	minWidth := p.heldWidth
	if p.menu != nil && (p.menu.wide || m.paletteOpen()) {
		minWidth = m.width * 3 / 4
	}

	return popupColumns(m.width, minWidth)
}

// popupLayout is p's rectangle and its lines, as fitPopups left them.
func (m *Model) popupLayout(p *popup) (x0, y0, x1, y1 int, lines []string) {
	x0, x1 = m.popupX(p)

	if p.menu != nil {
		lines = p.menu.list.rows
	} else {
		lines = strings.Split(ansi.Wrap(p.message, max(x1-x0-1, 1), ""), "\n")
	}
	y0, y1 = popupRows(m.height, len(lines))

	return x0, y0, x1, y1, lines
}

// fitPopups is pkg/gui's resizePopupPanel: a shorter subtitle mustn't
// narrow the popup under the cursor, so its width only grows.
func (m *Model) fitPopups() {
	for _, p := range m.popups {
		p.fittedSubtitle = fitBeside(p.subtitle, p.title, m.width-1)
		if p.fittedSubtitle != "" {
			p.heldWidth = max(p.heldWidth, utils.DisplayWidth(p.title)+utils.DisplayWidth(p.fittedSubtitle)+popupFrameMargin)
		}

		if p.menu != nil {
			x0, x1 := m.popupX(p)
			p.menu.list.fit(x1 - x0 - 1)
			y0, y1 := popupRows(m.height, len(p.menu.list.rows))
			p.menu.list.selectRow(p.menu.list.selected, y1-y0-1)
		}
	}
}
