package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/tallica/lazyincus/pkg/utils"
)

// Windows are pkg/gui's (window.go): number keys and cycling count them.

func (m *Model) visiblePanels() []*panel {
	return slices.DeleteFunc(slices.Clone(m.panels), (*panel).isHidden)
}

func (m *Model) panelNamed(name string) *panel {
	for _, p := range m.panels {
		if p.name == name {
			return p
		}
	}

	return nil
}

func (m *Model) isSideView(name string) bool {
	p := m.panelNamed(name)

	return p != nil && !p.isHidden()
}

func (m *Model) windowNames() []string {
	names := []string{}
	for _, p := range m.visiblePanels() {
		if !slices.Contains(names, p.windowName()) {
			names = append(names, p.windowName())
		}
	}

	return names
}

func (m *Model) windowPanels(window string) []*panel {
	return slices.DeleteFunc(m.visiblePanels(), func(p *panel) bool { return p.windowName() != window })
}

func (m *Model) windowOf(view string) string {
	if p := m.panelNamed(view); p != nil {
		return p.windowName()
	}

	return view
}

// activeInWindow is the window's last focused panel, else its first.
func (m *Model) activeInWindow(window string) *panel {
	panels := m.windowPanels(window)
	for _, p := range panels {
		if p.name == m.activeViews[window] {
			return p
		}
	}

	return panels[0]
}

// windowTitle is pkg/gui's: what the number key's description names.
func (m *Model) windowTitle(window string) string {
	switch window {
	case resourcesWindow:
		return m.tr.ResourcesTitle
	case snapshotsWindow:
		return m.tr.SnapshotsTitle
	}

	return m.windowPanels(window)[0].title()
}

func titlePrefix(index int) string {
	return fmt.Sprintf("[%d]", index+1)
}

// windowTabs is pkg/gui's fitWindowTabs for one window; nil for one panel.
func (m *Model) windowTabs(window string, index, width int) []string {
	panels := m.windowPanels(window)
	if len(panels) < 2 {
		return nil
	}

	active := m.activeInWindow(window)
	// Corners, the runes around the prefix, and gocui's rule before the corner.
	room := width - ansi.StringWidth(titlePrefix(index)) - 5

	titles := make([]string, len(panels))
	for i, p := range panels {
		titles[i] = p.title()
		if p != active && p.plainTitle != "" {
			titles[i] = p.plainTitle
		}
	}
	if ansi.StringWidth(strings.Join(titles, " - ")) > room {
		return fitActiveTab(panels, active, room)
	}

	return titles
}

// fitActiveTab is pkg/gui's fitActiveTab.
func fitActiveTab(panels []*panel, active *panel, room int) []string {
	short := make([]string, len(panels))
	titles := make([]string, len(panels))
	index := -1
	for i, p := range panels {
		short[i] = p.shortTitle
		titles[i] = p.shortTitle
		if p == active {
			titles[i] = p.title()
			index = i
		}
	}
	if index < 0 {
		return short
	}

	others := ansi.StringWidth(strings.Join(titles, " - ")) - ansi.StringWidth(titles[index])
	if room-others < ansi.StringWidth(active.shortTitle)+2 {
		return short
	}

	titles[index] = utils.Truncate(titles[index], room-others)

	return titles
}

// Focus is a stack of side panels and main, with popups stacked above.

func (m *Model) currentView() string {
	if len(m.popups) > 0 {
		// The palette's prompt has the focus over its menu.
		if m.paletteOpen() && m.topMenu() != nil {
			return filterView
		}
		return m.popups[len(m.popups)-1].view
	}

	return m.currentStaticView()
}

func (m *Model) currentStaticView() string {
	if len(m.focus) == 0 {
		return m.initialView()
	}

	return m.focus[len(m.focus)-1]
}

// currentSideView is the last side panel focused: what main shows.
func (m *Model) currentSideView() string {
	for i := len(m.focus) - 1; i >= 0; i-- {
		if m.isSideView(m.focus[i]) {
			return m.focus[i]
		}
	}

	return m.initialView()
}

func (m *Model) currentSidePanel() *panel {
	return m.panelNamed(m.currentSideView())
}

func (m *Model) initialView() string {
	return m.visiblePanels()[0].name
}

// focusView is pkg/gui's switchFocus.
func (m *Model) focusView(name string) {
	if m.stacksSwap(m.currentSideView(), name) {
		m.panelNamed(name).mainTab = 0
	}

	m.popups = nil
	if m.currentStaticView() == mainView && name != mainView {
		m.mainParent = ""
	}

	if m.isSideView(name) {
		m.focus = nil
		if window := m.windowOf(name); window != name {
			m.activeViews[window] = name
		}
	} else if len(m.focus) == 0 {
		// gocui focuses the initial view at startup, under whatever follows.
		m.focus = []string{m.initialView()}
	}

	m.focus = append(slices.DeleteFunc(m.focus, func(view string) bool { return view == name }), name)
	m.filterFollowsFocus()
}

func (m *Model) returnFocus() {
	if m.currentStaticView() == mainView {
		m.mainParent = ""
	}
	if len(m.focus) > 1 {
		m.focus = m.focus[:len(m.focus)-1]
	}
	m.filterFollowsFocus()
}

// stacksSwap is pkg/gui's stacksSwap.
func (m *Model) stacksSwap(from, to string) bool {
	if !m.stacksAway() {
		return false
	}

	stacks := func(name string) bool { return name == "stacks" || name == "services" }

	return (stacks(to) && !stacks(from)) || (to == "instances" && stacks(from))
}

// landOffStacks is pkg/gui's landOffStacks.
func (m *Model) landOffStacks() {
	if !m.stacksAway() {
		return
	}

	if view := m.currentSideView(); view == "stacks" || view == "services" {
		popups := m.popups
		m.focusView("instances")
		m.popups = popups
	}
}

// leaveHiddenView is pkg/gui's: the focus leaves a side panel that has just been hidden.
func (m *Model) leaveHiddenView() {
	p := m.panelNamed(m.currentSideView())
	if p == nil || !p.isHidden() {
		return
	}

	popups := m.popups
	m.focusView(m.activeInWindow(p.windowName()).name)
	m.popups = popups
}

// goToWindow is pkg/gui's handleGoToWindow.
func (m *Model) goToWindow(window string) {
	if len(m.popups) == 0 && m.windowOf(m.currentView()) == window {
		m.cycleWindowTab(1)
		return
	}

	m.focusView(m.activeInWindow(window).name)
}

func (m *Model) cycleWindowTab(offset int) {
	current := m.currentView()
	panels := m.windowPanels(m.windowOf(current))
	index := slices.IndexFunc(panels, func(p *panel) bool { return p.name == current })
	if index < 0 || len(panels) < 2 {
		return
	}

	m.focusView(panels[wrap(index+offset, len(panels))].name)
}

// cycleWindows is pkg/gui's cycleSidePanel.
func (m *Model) cycleWindows(offset int) {
	if len(m.popups) > 0 {
		return
	}

	names := m.windowNames()
	index := max(slices.Index(names, m.windowOf(m.currentSideView())), 0)

	m.focusView(m.activeInWindow(names[wrap(index+offset, len(names))]).name)
}

// cycleViews is pkg/gui's cycleSideView.
func (m *Model) cycleViews(offset int) {
	if len(m.popups) > 0 {
		return
	}

	panels := m.visiblePanels()
	index := max(slices.IndexFunc(panels, func(p *panel) bool { return p.name == m.currentSideView() }), 0)

	m.focusView(panels[wrap(index+offset, len(panels))].name)
}

func wrap(i, n int) int {
	return (i%n + n) % n
}
