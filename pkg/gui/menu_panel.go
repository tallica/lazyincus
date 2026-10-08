package gui

import (
	"strings"

	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

type CreateMenuOptions struct {
	Title string
	// Subtitle is a hint gocui right-aligns on the top border.
	Subtitle   string
	Items      []*types.MenuItem
	HideCancel bool
	// Selected is the row the cursor starts on.
	Selected int
	// Tabs, if any, stand in for the title, Tabs[TabIndex] the one on show.
	Tabs     []string
	TabIndex int
	// Wide is for rows with more to say than a menu's.
	Wide bool
}

func (gui *Gui) getMenuPanel() *panels.SideListPanel[*types.MenuItem] {
	return &panels.SideListPanel[*types.MenuItem]{
		ListPanel: panels.ListPanel[*types.MenuItem]{
			List: panels.NewFilteredList[*types.MenuItem](),
			View: gui.Views.Menu,
		},
		NoItemsMessage: "",
		Gui:            gui.intoInterface(),
		OnClick:        gui.onMenuPress,
		Sort:           nil,
		GetTableCells:  presentation.GetMenuItemDisplayStrings,
		OnRerender: func() error {
			return gui.resizePopupPanel(gui.Views.Menu)
		},
		DisableFilter: true,
		FuzzyFilter:   true,
		FilterText:    func(item *types.MenuItem) string { return item.FilterText },
		Filter: func(item *types.MenuItem) bool {
			return !item.HideUntilFiltered || gui.FilterString(gui.Views.Menu) != ""
		},
	}
}

func (gui *Gui) onMenuPress(menuItem *types.MenuItem) error {
	if err := gui.handleMenuClose(); err != nil {
		return err
	}

	if menuItem.OnPress != nil {
		return menuItem.OnPress()
	}

	return nil
}

func (gui *Gui) handleMenuPress() error {
	selectedMenuItem, err := gui.Panels.Menu.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.onMenuPress(selectedMenuItem)
}

func (gui *Gui) Menu(opts CreateMenuOptions) error {
	if !opts.HideCancel {
		opts.Items = append(opts.Items, &types.MenuItem{
			LabelColumns: []string{gui.Tr.Cancel},
			OnPress: func() error {
				return nil
			},
		})
	}

	maxColumnSize := 1

	for _, item := range opts.Items {
		if item.LabelColumns == nil {
			item.LabelColumns = []string{item.Label}
		}

		if item.OpensMenu {
			item.LabelColumns[0] = utils.OpensMenuStyle(item.LabelColumns[0])
		}

		maxColumnSize = utils.Max(maxColumnSize, len(item.LabelColumns))
	}

	for _, item := range opts.Items {
		if len(item.LabelColumns) < maxColumnSize {
			item.LabelColumns = append(item.LabelColumns, make([]string, maxColumnSize-len(item.LabelColumns))...)
		}
	}

	gui.State.DaemonPopup = false
	gui.State.WideMenu = opts.Wide
	// Set before the rows are rendered, which sizes the popup to them.
	gui.Views.Menu.Title = opts.Title
	// The title as well when there are tabs: gocui draws the tabs, and the
	// popup is sized by the title.
	if len(opts.Tabs) > 0 {
		gui.Views.Menu.Title = strings.Join(opts.Tabs, " - ")
	}
	gui.Views.Menu.Tabs = opts.Tabs
	gui.Views.Menu.TabIndex = opts.TabIndex
	gui.Views.Menu.Subtitle = opts.Subtitle

	gui.Panels.Menu.SetItems(opts.Items)
	gui.Panels.Menu.SetSelectedLineIdx(opts.Selected)

	if err := gui.Panels.Menu.RerenderList(); err != nil {
		return err
	}

	gui.Views.Menu.Visible = true

	return gui.switchFocus(gui.Views.Menu)
}

// specific functions

func (gui *Gui) renderMenuOptions() error {
	optionsMap := map[string]string{
		"esc":   gui.Tr.Close,
		"↑ ↓":   gui.Tr.Navigate,
		"enter": gui.Tr.Execute,
	}

	if gui.State.DaemonPopup {
		optionsMap["[ ]"] = gui.Tr.SwitchList
	}

	return gui.renderOptionsMap(optionsMap)
}

func (gui *Gui) handleMenuClose() error {
	gui.Views.Menu.Visible = false

	if gui.State.Filter.panel == gui.Panels.Menu {
		if err := gui.clearFilter(); err != nil {
			return err
		}

		gui.removeViewFromStack(gui.Views.Filter)
	}

	return gui.returnFocus()
}
