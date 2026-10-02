package gui

import (
	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
)

func (gui *Gui) newLineFocused(v *gocui.View) error {
	if v == nil {
		return nil
	}

	currentListPanel, ok := gui.currentListPanel()
	if ok {
		return currentListPanel.HandleSelect()
	}

	switch v.Name() {
	case "confirmation", "snapshotOptions":
		return nil
	case "main":
		v.Highlight = false
		return nil
	case "filter":
		return nil
	default:
		panic(gui.Tr.NoViewMachingNewLineFocusedSwitchStatement)
	}
}

// TODO: move some of this logic into our onFocusLost and onFocus hooks
func (gui *Gui) switchFocus(newView *gocui.View) error {
	gui.ViewStackMutex.Lock()
	defer gui.ViewStackMutex.Unlock()

	return gui.switchFocusAux(newView)
}

func (gui *Gui) switchFocusAux(newView *gocui.View) error {
	if gui.stacksSwap(gui.lastSideViewName(), newView.Name()) {
		if panel, ok := gui.sidePanelNamed(newView.Name()); ok {
			panel.SetMainTabIndex(0)
		}
	}

	gui.pushView(newView.Name())
	gui.noteActiveView(newView.Name())
	gui.Log.Info("setting highlight to true for view " + newView.Name())
	gui.Log.Info("new focused view is " + newView.Name())
	if _, err := gui.g.SetCurrentView(newView.Name()); err != nil {
		return err
	}

	gui.g.Cursor = newView.Editable

	if err := gui.renderPanelOptions(); err != nil {
		return err
	}

	newViewStack := gui.State.ViewStack

	if gui.State.Filter.panel != nil && !lo.Contains(newViewStack, gui.State.Filter.panel.GetView().Name()) {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	if !lo.Contains(newViewStack, "menu") {
		gui.Views.Menu.Visible = false
	}

	// A prompt can be more than one view - the snapshot popup is a name
	// field plus its options - and focus moving anywhere outside it, a
	// number key reaching the panels say, has to take the whole thing down.
	// Otherwise it sits there over a panel with no way back into it.
	if !lo.Contains(newViewStack, "confirmation") && !lo.Contains(newViewStack, "snapshotOptions") {
		gui.dismissPrompt()
	}

	return gui.newLineFocused(newView)
}

func (gui *Gui) returnFocus() error {
	gui.ViewStackMutex.Lock()
	defer gui.ViewStackMutex.Unlock()

	if len(gui.State.ViewStack) <= 1 {
		return nil
	}

	previousViewName := gui.State.ViewStack[len(gui.State.ViewStack)-2]
	previousView, err := gui.g.View(previousViewName)
	if err != nil {
		return err
	}

	return gui.switchFocusAux(previousView)
}

func (gui *Gui) removeViewFromStack(view *gocui.View) {
	gui.ViewStackMutex.Lock()
	defer gui.ViewStackMutex.Unlock()

	gui.State.ViewStack = lo.Filter(gui.State.ViewStack, func(viewName string, _ int) bool {
		return viewName != view.Name()
	})
}

// Not to be called directly. Use `switchFocus` instead
func (gui *Gui) pushView(name string) {
	if name != "filter" {
		gui.State.ViewStack = lo.Filter(gui.State.ViewStack, func(viewName string, _ int) bool {
			return !gui.isPopupPanel(viewName)
		})
	}

	if lo.Contains(gui.sideViewNames(), name) {
		gui.State.ViewStack = []string{}
	}

	gui.State.ViewStack = lo.Filter(gui.State.ViewStack, func(viewName string, _ int) bool {
		return viewName != name
	})

	gui.State.ViewStack = append(gui.State.ViewStack, name)
}

// excludes popups
func (gui *Gui) currentStaticViewName() string {
	gui.ViewStackMutex.Lock()
	defer gui.ViewStackMutex.Unlock()

	for i := len(gui.State.ViewStack) - 1; i >= 0; i-- {
		if !lo.Contains(gui.popupViewNames(), gui.State.ViewStack[i]) {
			return gui.State.ViewStack[i]
		}
	}

	return gui.initiallyFocusedViewName()
}

func (gui *Gui) currentSideViewName() string {
	gui.ViewStackMutex.Lock()
	defer gui.ViewStackMutex.Unlock()

	return gui.lastSideViewName()
}

// lastSideViewName is currentSideViewName for a caller holding
// ViewStackMutex.
func (gui *Gui) lastSideViewName() string {
	for idx := range gui.State.ViewStack {
		reversedIdx := len(gui.State.ViewStack) - 1 - idx
		viewName := gui.State.ViewStack[reversedIdx]
		if lo.Contains(gui.sideViewNames(), viewName) {
			return viewName
		}
	}

	return gui.initiallyFocusedViewName()
}

// stacksSwap is whether focus moving between these side views swaps Stacks
// and Services with Instances, on a remote with no stack: the panel landed
// on starts at its first main-panel tab.
func (gui *Gui) stacksSwap(from, to string) bool {
	if gui.State.StacksHere {
		return false
	}

	stacks := func(name string) bool { return name == "stacks" || name == "services" }

	return (stacks(from) && to == "instances") || (from == "instances" && stacks(to))
}
