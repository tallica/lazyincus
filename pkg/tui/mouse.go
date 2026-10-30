package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
)

// mouseAt is a cell; the left button and the wheel are all pkg/gui binds.
type mouseAt struct{ x, y int }

func (m *Model) onMouse(msg tea.MouseMsg) tea.Cmd {
	if m.cfg.UserConfig.Gui.IgnoreMouseEvents {
		return nil
	}

	mouse := msg.Mouse()
	at := mouseAt{mouse.X, mouse.Y}

	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch mouse.Button { //nolint:exhaustive // only the wheel scrolls
		case tea.MouseWheelUp:
			m.wheel(at, -1)
		case tea.MouseWheelDown:
			m.wheel(at, 1)
		}
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft {
			return m.click(at)
		}
	}

	return nil
}

func (at mouseAt) within(x0, y0, x1, y1 int) bool {
	return at.x >= x0 && at.x <= x1 && at.y >= y0 && at.y <= y1
}

func (m *Model) windowAt(at mouseAt) (string, boxlayout.Dimensions, bool) {
	dims := m.dims()
	for _, window := range append(m.windowNames(), mainView) {
		if d, ok := dims[window]; ok && at.within(d.X0, d.Y0, d.X1, d.Y1) {
			return window, d, true
		}
	}

	return "", boxlayout.Dimensions{}, false
}

// modal is ShouldHandleMouseEvent: a popup takes the mouse unless the filter prompt has the focus.
func (m *Model) modal() bool {
	return len(m.popups) > 0 && m.currentView() != filterView
}

func (m *Model) topMenuAt(at mouseAt) (*menu, int, int, bool) {
	if len(m.popups) == 0 {
		return nil, 0, 0, false
	}

	top := m.popups[len(m.popups)-1]
	x0, y0, x1, y1, _ := m.popupLayout(top)
	if top.menu == nil || !at.within(x0, y0, x1, y1) {
		return nil, 0, 0, false
	}

	return top.menu, y0, y1 - y0 - 1, true
}

// wheel is pkg/gui's: a list moves its selection, main scrolls.
func (m *Model) wheel(at mouseAt, step int) {
	if mn, _, _, ok := m.topMenuAt(at); ok {
		m.moveSelection(mn.list, step)
		return
	}
	if m.modal() {
		return
	}

	window, _, ok := m.windowAt(at)
	switch {
	case !ok:
	case window == mainView:
		m.scrollMain(step)
	default:
		m.moveSelection(m.activeInWindow(window), step)
	}
}

// click is pkg/gui's left click.
func (m *Model) click(at mouseAt) tea.Cmd {
	if _, _, _, ok := m.topMenuAt(at); ok {
		return m.clickMenu(at)
	}
	if m.modal() {
		if p := m.topPrompt(); p != nil {
			m.clickPrompt(p, at)
		}

		return nil
	}
	if m.clickFilter(at) {
		return nil
	}

	window, d, ok := m.windowAt(at)
	switch {
	case !ok:
	case window == mainView:
		m.clickMain(at, d)
	default:
		m.clickWindow(window, at, d)
	}

	return nil
}

// clickMenu is pkg/gui's menu OnClick.
func (m *Model) clickMenu(at mouseAt) tea.Cmd {
	mn, y0, rows, ok := m.topMenuAt(at)
	if !ok {
		return nil
	}

	mn.list.selectRow(mn.list.origin+at.y-y0-1, rows)

	return m.pressItem(mn)
}

// clickPrompt is gocui's SetCursor2D: the column counts along the text's first line, whatever the row.
func (m *Model) clickPrompt(p *popup, at mouseAt) {
	x0, y0, x1, y1, _ := m.popupLayout(p)
	if !at.within(x0, y0, x1, y1) {
		return
	}

	p.input.SetCursor(cursorAt(p.input.Value(), at.x-x0-1))
}

// clickFilter moves the prompt's cursor even unfocused, as gocui does for an editable view.
func (m *Model) clickFilter(at mouseAt) bool {
	d, ok := m.dims()[filterView]
	if !ok || !m.filter.active || !at.within(d.X0, d.Y0, d.X1, d.Y1) {
		return false
	}

	m.filter.input.SetCursor(cursorAt(m.filter.input.Value(), at.x-d.X0+m.filter.ox))

	return true
}

func cursorAt(value string, col int) int {
	position := 0
	for _, c := range cells(value) {
		if col <= 0 {
			break
		}
		col -= c.width
		position += len([]rune(c.chr))
	}

	return position
}

func (m *Model) clickWindow(window string, at mouseAt, d boxlayout.Dimensions) {
	p := m.activeInWindow(window)

	if at.y == d.Y0 {
		index := slices.Index(m.windowNames(), window)
		w, _ := size(d)
		tabs := m.windowTabs(window, index, w)
		if tab := clickedTab(titlePrefix(index), tabs, at.x-d.X0); tab >= 0 {
			m.focusView(m.windowPanels(window)[tab].name)
			return
		}
	}

	p.selectRow(p.origin+at.y-d.Y0-1, m.listRows(p))
	if m.currentView() != p.name {
		m.focusView(p.name)
	}
}

func (m *Model) clickMain(at mouseAt, d boxlayout.Dimensions) {
	p := m.currentSidePanel()

	if at.y == d.Y0 && p.mainTabs != nil && p.count() > 0 {
		tabs := p.mainTabs()
		titles := make([]string, len(tabs))
		for i, tab := range tabs {
			titles[i] = tab.title
		}
		if tab := clickedTab("", titles, at.x-d.X0); tab >= 0 {
			p.mainTab = tab
			return
		}
	}

	if m.currentView() != mainView {
		m.enterMain()
	}
}

// clickedTab is gocui's GetClickedTabIndex; a lone tab is 0 anywhere.
func clickedTab(prefix string, tabs []string, x int) int {
	if len(tabs) == 0 {
		return -1
	}
	if len(tabs) == 1 {
		return 0
	}

	edge := len(prefix) + 1
	if prefix != "" {
		edge++
	}
	if x <= edge {
		return -1
	}

	for i, tab := range tabs {
		edge += ansi.StringWidth(tab)
		if x <= edge {
			return i
		}

		edge += ansi.StringWidth(" - ")
		if x <= edge {
			return -1
		}
	}

	return -1
}
