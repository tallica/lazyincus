package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mouseClick(m *Model, x, y int) {
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	run(m, cmd)
}

func mouseWheel(m *Model, x, y int, button tea.MouseButton) {
	_, cmd := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
	run(m, cmd)
}

func TestClickSelectsRowAndFocuses(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	d := m.dims()["instances"]
	require.Greater(t, m.panelNamed("instances").count(), 2)

	mouseClick(m, d.X0+3, d.Y0+3)

	assert.Equal(t, "instances", m.currentView())
	assert.Equal(t, 2, m.panelNamed("instances").selected)
}

func TestClickBeyondLastRowSelectsLast(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 60)
	p := m.panelNamed("instances")
	d := m.dims()["instances"]
	require.Less(t, p.count(), d.Y1-d.Y0-1)

	mouseClick(m, d.X0+3, d.Y1-1)

	assert.Equal(t, p.count()-1, p.selected)
}

func TestWheelMovesSelectionWithoutFocus(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	d := m.dims()["instances"]
	before := m.currentView()

	mouseWheel(m, d.X0+3, d.Y0+2, tea.MouseWheelDown)
	assert.Equal(t, 1, m.panelNamed("instances").selected)
	mouseWheel(m, d.X0+3, d.Y0+2, tea.MouseWheelUp)
	assert.Equal(t, 0, m.panelNamed("instances").selected)
	assert.Equal(t, before, m.currentView())
}

func TestWheelScrollsMain(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 20)
	d := m.dims()[mainView]
	m.main.render(strings.Repeat("line\n", 100))

	mouseWheel(m, d.X0+3, d.Y0+3, tea.MouseWheelDown)

	assert.Equal(t, m.cfg.UserConfig.Gui.ScrollHeight, m.main.oy)
}

func TestClickMainFocusesItAndEnterReturns(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	d := m.dims()[mainView]

	mouseClick(m, d.X0+3, d.Y0+3)
	assert.Equal(t, mainView, m.currentView())
	assert.Equal(t, "instances", m.mainParent)

	press(m, "esc")
	assert.Equal(t, "instances", m.currentView())
}

func TestClickMainTabSwitchesTab(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	p := m.currentSidePanel()
	tabs := p.mainTabs()
	require.Greater(t, len(tabs), 1)
	d := m.dims()[mainView]

	x := d.X0 + 1 + len(tabs[0].title) + len(" - ") + 1
	mouseClick(m, x, d.Y0)

	assert.Equal(t, 1, p.mainTab)
	assert.Equal(t, "instances", m.currentView(), "a tab click leaves the focus where it was")
}

func TestClickWindowTabFocusesThatList(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	d := m.dims()[resourcesWindow]
	panels := m.windowPanels(resourcesWindow)
	require.Greater(t, len(panels), 1)
	tabs := m.windowTabs(resourcesWindow, slices.Index(m.windowNames(), resourcesWindow), d.X1-d.X0+1)

	x := d.X0 + len(titlePrefix(slices.Index(m.windowNames(), resourcesWindow))) + 2 + len(tabs[0]) + len(" - ") + 1
	mouseClick(m, x, d.Y0)

	assert.Equal(t, panels[1].name, m.currentView())
}

func TestPopupSwallowsOutsideClicks(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	press(m, "x")
	require.Len(t, m.popups, 1)

	mouseClick(m, 1, 3)
	mouseWheel(m, 1, 3, tea.MouseWheelDown)

	assert.Len(t, m.popups, 1)
	assert.Equal(t, 0, m.panelNamed("instances").selected)
	assert.Equal(t, 0, m.popups[0].menu.list.selected)
}

func TestClickMenuRowPressesIt(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	pressed := ""
	m.openMenu(menuOptions{title: "t", items: []*menuItem{
		{labelColumns: []string{"one"}, onPress: func(*Model) tea.Cmd { pressed = "one"; return nil }},
		{labelColumns: []string{"two"}, onPress: func(*Model) tea.Cmd { pressed = "two"; return nil }},
	}})
	x0, y0, _, _, _ := m.popupLayout(m.popups[0])

	mouseWheel(m, x0+2, y0+1, tea.MouseWheelDown)
	assert.Equal(t, 1, m.popups[0].menu.list.selected)

	_, y0, _, _, _ = m.popupLayout(m.popups[0])
	mouseClick(m, x0+2, y0+1)

	assert.Equal(t, "one", pressed)
	assert.Empty(t, m.popups)
}

func TestIgnoreMouseEvents(t *testing.T) {
	m := start(t, options{fixture: true}, 120, 40)
	m.cfg.UserConfig.Gui.IgnoreMouseEvents = true
	d := m.dims()["instances"]

	mouseClick(m, d.X0+3, d.Y0+3)

	assert.Equal(t, 0, m.panelNamed("instances").selected)
	assert.Equal(t, tea.MouseModeNone, m.View().MouseMode)
}

func TestClickedTab(t *testing.T) {
	tabs := []string{"Images", "Volumes"}
	assert.Equal(t, -1, clickedTab("[5]", tabs, 5))
	assert.Equal(t, 0, clickedTab("[5]", tabs, 6))
	assert.Equal(t, 0, clickedTab("[5]", tabs, 11))
	assert.Equal(t, -1, clickedTab("[5]", tabs, 14))
	assert.Equal(t, 1, clickedTab("[5]", tabs, 15))
	assert.Equal(t, 0, clickedTab("", []string{"Logs"}, 40))
	assert.Equal(t, -1, clickedTab("", nil, 3))
}

func TestClickInAPromptMovesTheCursor(t *testing.T) {
	m := start(t, options{}, 140, 40)
	openPrompt(m, "héllo wörld")
	x0, y0, x1, _, _ := m.popupLayout(m.popups[0])

	mouseClick(m, x0+1+4, y0+1)
	assert.Equal(t, 4, m.popups[0].input.Position())

	mouseClick(m, x1-1, y0+1)
	assert.Equal(t, 11, m.popups[0].input.Position(), "past the end is the end")

	mouseClick(m, x0, y0)
	assert.Equal(t, 0, m.popups[0].input.Position(), "the border is the start")

	typeText(m, "X")
	assert.Equal(t, "Xhéllo wörld", m.popups[0].input.Value())
}

func TestClickOutsideAPromptDoesNothing(t *testing.T) {
	m := start(t, options{}, 140, 40)
	openPrompt(m, "db")
	before := m.popups[0].input.Position()

	mouseClick(m, 0, 0)
	assert.Equal(t, before, m.popups[0].input.Position())
	assert.Len(t, m.popups, 1)
}
