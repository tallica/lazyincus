package gui

import (
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/utils"
)

// A window is a slot in the layout. Most side panels have one to themselves;
// the resources window holds several, stacked at the same position with
// only the active one visible, and its title is their tabs. Number keys,
// tab cycling and the side column's split all count windows, not views.

const resourcesWindow = "resources"

// sideWindowNames are the visible side panels' windows, top to bottom.
func (gui *Gui) sideWindowNames() []string {
	return lo.Uniq(lo.Map(gui.visibleSidePanelDefs(), func(def sidePanelDef, _ int) string {
		return def.windowName()
	}))
}

// windowDefs are the side panels sharing a window, in tab order.
func (gui *Gui) windowDefs(window string) []sidePanelDef {
	return lo.Filter(gui.visibleSidePanelDefs(), func(def sidePanelDef, _ int) bool {
		return def.windowName() == window
	})
}

// windowOfView is the window a view is drawn in: its own name, unless it's a
// side panel sharing one.
func (gui *Gui) windowOfView(viewName string) string {
	def, ok := lo.Find(gui.sidePanelDefs(), func(def sidePanelDef) bool { return def.name == viewName })
	if !ok {
		return viewName
	}

	return def.windowName()
}

// activeViewInWindow is the view a window shows: the one last focused in it,
// else its first.
func (gui *Gui) activeViewInWindow(window string) string {
	if name, ok := gui.State.ActiveWindowViews[window]; ok {
		return name
	}

	return gui.windowDefs(window)[0].name
}

// noteActiveView records a focused side view as its window's active one.
func (gui *Gui) noteActiveView(viewName string) {
	window := gui.windowOfView(viewName)
	if window == viewName {
		return
	}

	if gui.State.ActiveWindowViews == nil {
		gui.State.ActiveWindowViews = map[string]string{}
	}

	gui.State.ActiveWindowViews[window] = viewName
}

// isHiddenInWindow reports a side view that shares its window and isn't the
// one it shows.
func (gui *Gui) isHiddenInWindow(viewName string) bool {
	window := gui.windowOfView(viewName)

	return window != viewName && gui.activeViewInWindow(window) != viewName
}

func (gui *Gui) windowTitle(window string) string {
	if window == resourcesWindow {
		return gui.Tr.ResourcesTitle
	}

	return gui.windowDefs(window)[0].title
}

// fitWindowTabs gives a shared window's views their tab names, short ones
// when the full ones would run past the title: a tab cut off the end is a
// list nobody knows is there. The layout calls it, so a resize refits.
func (gui *Gui) fitWindowTabs() {
	for index, window := range gui.sideWindowNames() {
		defs := gui.windowDefs(window)
		if len(defs) < 2 {
			continue
		}

		titles := lo.Map(defs, func(def sidePanelDef, _ int) string { return def.title })

		// The frame's corner and rune either side of the prefix.
		room := (*defs[0].viewPtr).Width() - utils.DisplayWidth(gui.sidePanelTitlePrefix(index)) - 4
		if utils.DisplayWidth(strings.Join(titles, " - ")) > room {
			titles = lo.Map(defs, func(def sidePanelDef, _ int) string { return def.shortTitle })
		}

		for _, def := range defs {
			(*def.viewPtr).Tabs = titles
		}
	}
}

// handleGoToWindow focuses a window's active view, or its next tab when the
// window has focus already, so its number key pressed again cycles through
// its lists.
func (gui *Gui) handleGoToWindow(window string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if gui.windowOfView(gui.currentViewName()) == window {
			return gui.cycleWindowTab(1)()
		}

		view, err := gui.g.View(gui.activeViewInWindow(window))
		if err != nil {
			return err
		}

		gui.resetMainView()

		return gui.switchFocus(view)
	}
}

// cycleWindowTab moves to the next (offset 1) or previous (offset -1) list in
// the focused window, wrapping at both ends.
func (gui *Gui) cycleWindowTab(offset int) func() error {
	return func() error {
		current := gui.currentViewName()
		defs := gui.windowDefs(gui.windowOfView(current))

		index := lo.IndexOf(lo.Map(defs, func(def sidePanelDef, _ int) string { return def.name }), current)
		if index < 0 || len(defs) < 2 {
			return nil
		}

		return gui.switchFocus(*defs[((index+offset)%len(defs)+len(defs))%len(defs)].viewPtr)
	}
}

// onWindowTabClick is a click on one of a window's tab titles.
func (gui *Gui) onWindowTabClick(window string) func(int) error {
	return func(tabIndex int) error {
		defs := gui.windowDefs(window)
		if tabIndex < 0 || tabIndex >= len(defs) {
			return nil
		}

		return gui.switchFocus(*defs[tabIndex].viewPtr)
	}
}

// excludes popups
func (gui *Gui) currentStaticWindowName() string {
	return gui.windowOfView(gui.currentStaticViewName())
}

func (gui *Gui) currentSideWindowName() string {
	return gui.windowOfView(gui.currentSideViewName())
}
