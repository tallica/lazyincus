package gui

import (
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
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
	if err := gui.Menu(CreateMenuOptions{
		Subtitle:    gui.daemonHint(tab),
		Items:       gui.daemonItems(tab),
		HideCancel:  true,
		Tabs:        gui.daemonTabs(),
		TabIndex:    int(tab),
		Wide:        true,
		FlexColumns: gui.daemonFlexColumns(tab),
	}); err != nil {
		return err
	}

	gui.State.DaemonPopup, gui.State.DaemonTab, gui.State.DaemonOpened = true, tab, true

	if tab == daemonOperations {
		gui.seeOperations()
	}

	// Rendered again now the popup is the daemon's: Menu filtered the rows
	// as any menu's, which ranks them by how well they match.
	gui.Panels.Menu.SetSelectedLineIdx(max(gui.daemonRow(selected), 0))
	if err := gui.Panels.Menu.RerenderList(); err != nil {
		return err
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

// handleDaemonFilter is `/` in the popup.
func (gui *Gui) handleDaemonFilter(*gocui.Gui, *gocui.View) error {
	if !gui.State.DaemonPopup {
		return nil
	}

	gui.State.Filter.active = true
	gui.State.Filter.panel = gui.Panels.Menu
	gui.renderFilterPrompt()

	return gui.switchFocus(gui.Views.Filter)
}

// daemonTabs names the operations after what they're cut down to.
func (gui *Gui) daemonTabs() []string {
	operations := gui.Tr.OperationsTitle
	if shown := gui.operationsShownLabel(); shown != "" {
		operations += " (" + shown + ")"
	}

	return []string{operations, gui.Tr.WarningsTitle}
}

func (gui *Gui) daemonHint(tab daemonTab) string {
	if tab == daemonOperations {
		return gui.Tr.OperationsHint
	}

	return gui.Tr.WarningsHint
}

func (gui *Gui) daemonItems(tab daemonTab) []*types.MenuItem {
	if tab == daemonOperations {
		return gui.operationItems()
	}

	return gui.warningItems()
}

func (gui *Gui) daemonFlexColumns(tab daemonTab) []utils.FlexColumn {
	if tab == daemonOperations {
		return presentation.OperationFlexColumns(
			gui.State.SpansProjects.Operations, presentation.OperationTookWidth(gui.operations.list()))
	}

	return nil
}

// daemonRow is the index of the row keyed key among those shown, or -1.
func (gui *Gui) daemonRow(key string) int {
	if key == "" {
		return -1
	}

	return gui.Panels.Menu.List.GetIndexBy(func(item *types.MenuItem) bool { return item.Key == key })
}

// daemonSelected is the key of the row the popup has selected on tab, or
// "" when it isn't open there. Read it before the state behind the rows
// changes: redrawDaemon puts the cursor back on that row.
func (gui *Gui) daemonSelected(tab daemonTab) string {
	if !gui.daemonOpen(tab) {
		return ""
	}

	// On the first row, the cursor follows the newest.
	if tab == daemonOperations && gui.Panels.Menu.SelectedIdx == 0 {
		return ""
	}

	item, err := gui.Panels.Menu.GetSelectedItem()
	if err != nil {
		return ""
	}

	return item.Key
}

// redrawDaemon redraws the popup in place if it's open on tab, the cursor
// on the row keyed selected: the menu is focused already, and opening it
// again would stack it on itself. Left where it was, the cursor would be on
// another row once a refresh has moved them, and the next key act on it.
func (gui *Gui) redrawDaemon(tab daemonTab, selected string) error {
	if !gui.daemonOpen(tab) {
		return nil
	}

	was := gui.Panels.Menu.SelectedIdx
	gui.Views.Menu.Tabs = gui.daemonTabs()
	gui.Views.Menu.Title = strings.Join(gui.Views.Menu.Tabs, " - ")
	gui.State.MenuFlexColumns = gui.daemonFlexColumns(tab)
	gui.Panels.Menu.SetItems(gui.daemonItems(tab))

	index := max(gui.daemonRow(selected), 0)
	gui.Panels.Menu.SetSelectedLineIdx(index)

	if tab == daemonOperations {
		// The rows on screen stay put as new ones arrive above them.
		if ox, oy := gui.Views.Menu.Origin(); index != was {
			gui.Views.Menu.SetOrigin(ox, max(oy+gui.Panels.Menu.SelectedIdx-was, 0))
		}

		gui.seeOperations()
	}

	return gui.Panels.Menu.RerenderList()
}
