package gui

import (
	"github.com/jesseduffield/gocui"
)

const UNKNOWN_VIEW_ERROR_MSG = "unknown view"

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

func (gui *Gui) onFocusChange() error {
	currentView := gui.g.CurrentView()
	for _, view := range gui.g.Views() {
		view.Highlight = view == currentView && view.Name() != "main"
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
		_, err := setViewFromDimensions(viewName, viewName)
		if err != nil && err.Error() != UNKNOWN_VIEW_ERROR_MSG {
			return err
		}
	}

	return gui.resizeCurrentPopupPanel(g)
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
