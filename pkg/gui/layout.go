package gui

import (
	"errors"

	"github.com/jesseduffield/gocui"
)

// getFocusLayout returns a manager function for when view gain and lose focus
func (gui *Gui) getFocusLayout() func(g *gocui.Gui) error {
	var previousView *gocui.View
	return func(g *gocui.Gui) error {
		newView := gui.g.CurrentView()
		if err := gui.onFocusChange(); err != nil {
			return err
		}
		// for now we don't consider losing focus to a popup panel as actually losing focus
		if newView != previousView && !gui.isPopupPanel(newView.Name()) {
			gui.onFocusLost(previousView, newView)
			gui.onFocus(newView)
			previousView = newView
		}
		return nil
	}
}

// onFocusChange highlights the focused view's selection, and the selection
// of the list the main panel is showing while something else has the focus,
// in the inactive style.
func (gui *Gui) onFocusChange() error {
	currentView := gui.g.CurrentView()
	sideView := gui.currentSideViewName()
	for _, view := range gui.g.Views() {
		// The menu's inactive highlight is its own: see styleAllViews.
		if view == gui.Views.Menu {
			view.Highlight = true
			continue
		}

		focused := view == currentView && view.Name() != "main"
		view.HighlightInactive = !focused && view.Name() == sideView
		view.Highlight = focused || view.HighlightInactive
	}
	return nil
}

func (gui *Gui) onFocusLost(v *gocui.View, newView *gocui.View) {
	if v == nil {
		return
	}

	if !gui.isPopupPanel(newView.Name()) {
		v.ParentView = nil
	}

	gui.focusPointInView(v)

	gui.Log.Info(v.Name() + " focus lost")
}

func (gui *Gui) onFocus(v *gocui.View) {
	if v == nil {
		return
	}

	gui.focusPointInView(v)

	if v.Name() == "operations" {
		gui.seeOperations()
	}

	gui.Log.Info(v.Name() + " focus gained")
}

// layout is called for every screen re-render e.g. when the screen is resized
func (gui *Gui) layout(g *gocui.Gui) error {
	g.Highlight = true
	width, height := g.Size()

	appStatus := gui.statusManager.getStatusString()

	viewDimensions := gui.getWindowDimensions(gui.getInformationContent(), appStatus)
	// we assume that the view has already been created.
	setViewFromDimensions := func(viewName string, windowName string) (*gocui.View, error) {
		dimensionsObj, ok := viewDimensions[windowName]

		view, err := g.View(viewName)
		if err != nil {
			return nil, err
		}

		if !ok {
			_, err := g.SetView(viewName, 0, 0, width, height, 0)
			view.Visible = false
			return view, err
		}

		frameOffset := 1
		if view.Frame {
			frameOffset = 0
		}
		_, err = g.SetView(
			viewName,
			dimensionsObj.X0-frameOffset,
			dimensionsObj.Y0-frameOffset,
			dimensionsObj.X1+frameOffset,
			dimensionsObj.Y1+frameOffset,
			0,
		)
		view.Visible = true

		return view, err
	}

	for _, viewName := range gui.autoPositionedViewNames() {
		view, err := setViewFromDimensions(viewName, gui.windowOfView(viewName))
		if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		if view != nil && gui.isHiddenInWindow(viewName) {
			view.Visible = false
		}
	}

	gui.fitWindowTabs()
	gui.titleStacks()
	gui.colorRemoteFrames()

	if gui.Views.Main != nil {
		mainWidth := gui.Views.Main.InnerWidth()
		gui.mainViewWidth.Store(int32(mainWidth))
	}

	if err := gui.resizeCurrentPopupPanel(g); err != nil {
		return err
	}

	// Last, so rows are cut to the width they're about to be drawn at.
	for _, panel := range gui.allListPanels() {
		panel.FitToWidth()
	}

	return nil
}

func (gui *Gui) focusPointInView(view *gocui.View) {
	if view == nil {
		return
	}

	for _, panel := range gui.allListPanels() {
		if panel.GetView() == view {
			panel.Refocus()
			return
		}
	}
}

func (gui *Gui) prepareView(viewName string) (*gocui.View, error) {
	return gui.g.SetView(viewName, 0, 0, 10, 10, 0)
}
