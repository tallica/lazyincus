package gui

import (
	"slices"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// daemonTab is a list in the popup `W` opens: what the daemon is doing and
// what it has noticed, both the remote's rather than any item's.
type daemonTab int

const (
	daemonOperations daemonTab = iota
	daemonWarnings
)

// daemonOpen reports the popup on screen being the daemon's, on tab.
func (gui *Gui) daemonOpen(tab daemonTab) bool {
	return gui.State.DaemonPopup && gui.State.DaemonTab == tab && gui.Views.Menu.Visible
}

// handleOpenDaemon is `W`: the tab last looked at, or the first time the
// warnings if any are new, from what the last poll found, each read again
// in the background, which redraws it.
func (gui *Gui) handleOpenDaemon(g *gocui.Gui, v *gocui.View) error {
	tab := gui.State.DaemonTab
	if !gui.State.DaemonOpened && gui.newWarnings.Load() > 0 {
		tab = daemonWarnings
	}

	if err := gui.openDaemon(tab, ""); err != nil {
		return err
	}

	gui.refreshInBackground(gui.fetchOperations, gui.fetchWarnings)

	return nil
}

// openDaemon opens the popup on tab, the cursor on the row keyed selected,
// or the first.
func (gui *Gui) openDaemon(tab daemonTab, selected string) error {
	items, keys := gui.daemonItems(tab)

	if err := gui.Menu(CreateMenuOptions{
		Subtitle:   gui.daemonHint(tab),
		Items:      items,
		HideCancel: true,
		Selected:   max(slices.Index(keys, selected), 0),
		Tabs:       []string{gui.Tr.OperationsTitle, gui.Tr.WarningsTitle},
		TabIndex:   int(tab),
		Wide:       true,
	}); err != nil {
		return err
	}

	gui.State.DaemonPopup, gui.State.DaemonTab, gui.State.DaemonOpened = true, tab, true

	if tab == daemonOperations {
		gui.seeOperations()
	}

	return gui.renderMenuOptions()
}

// handleDaemonTab is `[` and `]` in the popup.
func (gui *Gui) handleDaemonTab(step int) func(*gocui.Gui, *gocui.View) error {
	return func(*gocui.Gui, *gocui.View) error {
		if !gui.State.DaemonPopup {
			return nil
		}

		tabs := 2

		return gui.openDaemon(daemonTab((int(gui.State.DaemonTab)+step+tabs)%tabs), "")
	}
}

func (gui *Gui) daemonHint(tab daemonTab) string {
	if tab == daemonOperations {
		return gui.Tr.OperationsHint
	}

	return gui.Tr.WarningsHint
}

// daemonItems are tab's rows, and the key of the item on each.
func (gui *Gui) daemonItems(tab daemonTab) ([]*types.MenuItem, []string) {
	if tab == daemonOperations {
		return gui.operationItems()
	}

	return gui.warningItems()
}

// daemonSelected is the key of the row the popup has selected on tab, or
// "" when it isn't open there. Read it before the state behind the rows
// changes: redrawDaemon puts the cursor back on that row.
func (gui *Gui) daemonSelected(tab daemonTab) string {
	if !gui.daemonOpen(tab) {
		return ""
	}

	_, keys := gui.daemonItems(tab)
	if index := gui.Panels.Menu.SelectedIdx; index >= 0 && index < len(keys) {
		return keys[index]
	}

	return ""
}

// redrawDaemon redraws the popup in place if it's open on tab, the cursor
// on the row keyed selected: the menu is focused already, and opening it
// again would stack it on itself. Left where it was, the cursor would be on
// another row once a refresh has moved them, and the next key act on it.
func (gui *Gui) redrawDaemon(tab daemonTab, selected string) error {
	if !gui.daemonOpen(tab) {
		return nil
	}

	items, keys := gui.daemonItems(tab)
	gui.Panels.Menu.SetItems(items)

	if index := slices.Index(keys, selected); index >= 0 {
		gui.Panels.Menu.SetSelectedLineIdx(index)
	}

	if tab == daemonOperations {
		gui.seeOperations()
	}

	return gui.Panels.Menu.RerenderList()
}
