package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

const (
	menuView         = "menu"
	confirmationView = "confirmation"
)

// menuItem is pkg/gui's types.MenuItem with handlers that return a Cmd.
type menuItem struct {
	label        string
	labelColumns []string
	onPress      func(*Model) tea.Cmd
	opensMenu    bool
	// hideUntilFiltered keeps the item out until the palette's filter has text.
	hideUntilFiltered bool
	keys              map[string]menuKey
	checked           bool
}

// menuKey is pkg/gui's types.MenuKey.
type menuKey struct {
	handler func(*Model) tea.Cmd
	mutates bool
}

// menuOptions is pkg/gui's CreateMenuOptions.
type menuOptions struct {
	title      string
	items      []*menuItem
	hideCancel bool
	selected   int
	wide       bool
	// onConfirm makes the menu a checklist, handed the rows ticked on enter.
	onConfirm func(picked []*menuItem) tea.Cmd
}

type menu struct {
	menuOptions
	list *panel
}

// openMenu is pkg/gui's Menu.
func (m *Model) openMenu(opts menuOptions) {
	if !opts.hideCancel && opts.onConfirm == nil {
		opts.items = append(opts.items, &menuItem{labelColumns: []string{m.tr.Cancel}, onPress: func(*Model) tea.Cmd { return nil }})
	}

	columns := 1
	for _, item := range opts.items {
		if item.labelColumns == nil {
			item.labelColumns = []string{item.label}
		}
		if item.opensMenu {
			item.labelColumns[0] = utils.OpensMenuStyle(item.labelColumns[0])
		}
		columns = max(columns, len(item.labelColumns))
	}
	for _, item := range opts.items {
		item.labelColumns = append(item.labelColumns, make([]string, columns-len(item.labelColumns))...)
	}

	mn := &menu{menuOptions: opts, list: &panel{name: menuView}}
	mn.list.keep = func(i int) bool { return !mn.items[i].hideUntilFiltered || m.menuNeedle() != "" }
	mn.list.selected = opts.selected
	m.popups = append(m.popups, &popup{view: menuView, title: opts.title, menu: mn})
	m.rerenderMenu()
}

// menuNeedle is pkg/gui's FilterString for the menu: the palette's.
func (m *Model) menuNeedle() string {
	if m.filter.panel != menuView {
		return ""
	}

	return m.filter.input.Value()
}

func (m *Model) topMenu() *menu {
	if len(m.popups) == 0 {
		return nil
	}

	return m.popups[len(m.popups)-1].menu
}

// rerenderMenu is the menu panel's RerenderList.
func (m *Model) rerenderMenu() {
	mn := m.topMenu()
	if mn == nil {
		return
	}

	checklist := mn.onConfirm != nil
	mn.list.cells = make([][]string, len(mn.items))
	for i, item := range mn.items {
		shown := &types.MenuItem{LabelColumns: item.labelColumns, Checked: item.checked}
		mn.list.cells[i] = presentation.GetMenuItemDisplayStrings(shown, checklist)
	}
	m.filterList(mn.list)
}

func (mn *menu) selectedItem() *menuItem {
	if i := mn.list.item(); i >= 0 {
		return mn.items[i]
	}

	return nil
}

// menuBindings are pkg/gui's on the menu view.
func (m *Model) menuBindings() []*binding {
	return []*binding{
		{view: menuView, key: "esc", handler: do((*Model).closeMenu)},
		{view: menuView, key: "q", handler: do((*Model).closeMenu)},
		{view: menuView, key: "space", handler: (*Model).toggleMenuItem},
		{view: menuView, key: "enter", handler: (*Model).pressMenu},
		{view: menuView, key: "y", handler: (*Model).pressMenu},
		{view: menuView, key: "a", handler: func(m *Model) tea.Cmd {
			if mn := m.topMenu(); mn != nil && mn.onConfirm != nil {
				m.toggleAllMenuItems()
				return nil
			}

			return m.menuItemKey("a")
		}},
		{view: menuView, key: "d", handler: func(m *Model) tea.Cmd { return m.menuItemKey("d") }},
		{view: menuView, key: "o", handler: func(m *Model) tea.Cmd { return m.menuItemKey("o") }},
	}
}

// closeMenu is pkg/gui's handleMenuClose.
func (m *Model) closeMenu() {
	m.closePopup()
}

// pressMenu is pkg/gui's handleMenuPress.
func (m *Model) pressMenu() tea.Cmd {
	mn := m.topMenu()

	if mn.onConfirm != nil {
		picked := slices.DeleteFunc(slices.Clone(mn.items), func(item *menuItem) bool { return !item.checked })
		m.closeMenu()
		if len(picked) == 0 {
			return nil
		}

		return mn.onConfirm(picked)
	}

	item := mn.selectedItem()
	if item == nil {
		return nil
	}

	m.closeMenu()
	if item.onPress == nil {
		return nil
	}

	return item.onPress(m)
}

// toggleMenuItem is pkg/gui's handleMenuToggle: a tick, or in a plain menu a press.
func (m *Model) toggleMenuItem() tea.Cmd {
	mn := m.topMenu()
	if mn.onConfirm == nil {
		return m.pressMenu()
	}

	if item := mn.selectedItem(); item != nil {
		item.checked = !item.checked
		m.rerenderMenu()
	}

	return nil
}

func (m *Model) toggleAllMenuItems() {
	mn := m.topMenu()
	all := !slices.ContainsFunc(mn.items, func(item *menuItem) bool { return !item.checked })
	for _, item := range mn.items {
		item.checked = !all
	}
	m.rerenderMenu()
}

// menuItemKey is pkg/gui's handleMenuKey, guarded on the session's remote.
func (m *Model) menuItemKey(key string) tea.Cmd {
	item := m.topMenu().selectedItem()
	if item == nil {
		return nil
	}

	k, ok := item.keys[key]
	if !ok {
		return nil
	}

	if remote := m.incus.RemoteName(); k.mutates && m.isReadOnly(remote) {
		if m.cfg.ReadOnly {
			m.openError(m.tr.ReadOnlySession)
			return nil
		}

		m.openError(fmt.Sprintf(m.tr.ReadOnlyRemote, remote))
		return nil
	}

	return k.handler(m)
}

// menuOptionsMap is pkg/gui's renderMenuOptions.
func (m *Model) menuOptionsMap(mn *menu) map[string]string {
	options := map[string]string{"esc": m.tr.Close, "↑ ↓": m.tr.Navigate, "enter": m.tr.Execute}

	if mn.onConfirm != nil {
		options["space"] = m.tr.Toggle
		options["a"] = m.tr.ToggleAll
		options["enter"] = m.tr.Apply
	}

	if slices.ContainsFunc(mn.items, func(item *menuItem) bool { return item.keys["o"].handler != nil }) {
		options["o"] = m.tr.OpenWhatChanged
	}

	return options
}

// optionsBindings is pkg/gui's getBindings: the view's, a gap, the globals.
func (m *Model) optionsBindings(view string) []*binding {
	var global, own []*binding

	bindings := m.bindings()
	for _, b := range bindings {
		if !b.listed() {
			continue
		}
		switch b.view {
		case "":
			global = append(global, b)
		case view:
			own = append(own, b)
		}
	}

	if view == mainView && m.mainParent != "" {
		for _, b := range bindings {
			if !b.listed() || b.view != m.mainParent {
				continue
			}
			if b.key != "" && slices.ContainsFunc(own, func(o *binding) bool { return o.label() == b.label() }) {
				continue
			}
			own = append(own, b)
		}
	}

	return append(append(own, &binding{}), global...)
}

// openOptionsMenu is pkg/gui's handleCreateOptionsMenu.
func (m *Model) openOptionsMenu() {
	if len(m.popups) > 0 {
		return
	}

	bindings := m.optionsBindings(m.currentView())
	items := make([]*menuItem, len(bindings))
	for i, b := range bindings {
		items[i] = &menuItem{labelColumns: []string{b.label(), b.description}, onPress: b.handler}
	}

	m.openMenu(menuOptions{title: m.tr.MenuTitle, items: items, hideCancel: true})
}
