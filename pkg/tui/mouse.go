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

// topMenuAt: a popup takes every click and wheel, one outside it nothing.
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
	if len(m.popups) > 0 {
		if mn, _, _, ok := m.topMenuAt(at); ok {
			m.moveSelection(mn.list, step)
		}

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
	if len(m.popups) > 0 {
		return m.clickMenu(at)
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
