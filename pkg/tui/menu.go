package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

const (
	menuView         = "menu"
	confirmationView = "confirmation"
)

// menuItem is a menu row with handlers that return a Cmd.
type menuItem struct {
	// key: W's popup keeps the cursor on it.
	key          string
	label        string
	labelColumns []string
	onPress      func(*Model) tea.Cmd
	opensMenu    bool
	// filterText is what the palette's filter matches; the columns when "".
	filterText string
	// hideUntilFiltered keeps the item out until the palette's filter has text.
	hideUntilFiltered bool
	onTab             func(*Model) tea.Cmd
	keys              map[string]menuKey
	checked           bool
}

type menuKey struct {
	handler func(*Model) tea.Cmd
	mutates bool
}

type menuOptions struct {
	title      string
	subtitle   string
	items      []*menuItem
	hideCancel bool
	selected   int
	wide       bool
	// tabs, if any, stand in for the title, tabs[tabIndex] the one on show.
	tabs        []string
	tabIndex    int
	flexColumns []utils.FlexColumn
	// daemon: the menu is W's.
	daemon bool
	// onConfirm makes the menu a checklist, handed the rows ticked on enter.
	onConfirm func(picked []*menuItem) tea.Cmd
}

type menu struct {
	menuOptions
	list *panel
}

func (m *Model) openMenu(opts menuOptions) {
	if !opts.hideCancel && opts.onConfirm == nil {
		opts.items = append(opts.items, &menuItem{labelColumns: []string{m.tr.Cancel}, onPress: func(*Model) tea.Cmd { return nil }})
	}
	padMenuItems(opts.items)

	mn := &menu{menuOptions: opts, list: &panel{name: menuView, fuzzy: true, keepsOrder: opts.daemon}}
	mn.list.keep = func(i int) bool { return !mn.items[i].hideUntilFiltered || m.menuNeedle() != "" }
	mn.list.filterText = func(i int) string { return mn.items[i].filterText }
	mn.list.flex = func() []utils.FlexColumn { return mn.flexColumns }
	// Or a filter matching nothing would shrink W's popup to its frame.
	mn.list.note = func() string {
		if mn.daemon && m.menuNeedle() != "" {
			return m.tr.NoMatches
		}

		return ""
	}
	mn.list.selected = opts.selected

	// A menu opened replaces the one open.
	m.popups = slices.DeleteFunc(m.popups, func(p *popup) bool { return p.menu != nil })

	title := opts.title
	if len(opts.tabs) > 0 {
		title = strings.Join(opts.tabs, " - ")
	}
	m.popups = append(m.popups, &popup{view: menuView, title: title, subtitle: opts.subtitle, menu: mn})
	m.renderMenu(mn)
}

func padMenuItems(items []*menuItem) {
	columns := 1
	for _, item := range items {
		if item.labelColumns == nil {
			item.labelColumns = []string{item.label}
		}
		if item.opensMenu {
			item.labelColumns[0] = utils.OpensMenuStyle(item.labelColumns[0])
		}
		columns = max(columns, len(item.labelColumns))
	}
	for _, item := range items {
		item.labelColumns = append(item.labelColumns, make([]string, columns-len(item.labelColumns))...)
	}
}

// menuNeedle is the text the menu filters by: the palette's.
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

func (m *Model) rerenderMenu() {
	if mn := m.topMenu(); mn != nil {
		m.renderMenu(mn)
	}
}

func (m *Model) renderMenu(mn *menu) {
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

// menuBindings are the keys of the menu.
func (m *Model) menuBindings() []*binding {
	return []*binding{
		{view: menuView, key: "esc", handler: do((*Model).escapeMenu)},
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
		{view: menuView, key: "[", handler: do(func(m *Model) { m.switchDaemonTab(-1) })},
		{view: menuView, key: "]", handler: do(func(m *Model) { m.switchDaemonTab(1) })},
		{view: menuView, key: "/", handler: do((*Model).openDaemonFilter)},
		{view: menuView, key: "f", handler: do((*Model).cycleOperationsShown)},
	}
}

// escapeMenu closes W's filter before the popup.
func (m *Model) escapeMenu() {
	if mn := m.topMenu(); mn != nil && mn.daemon && m.menuNeedle() != "" {
		m.clearFilter()
		return
	}

	m.closeMenu()
}

func (m *Model) closeMenu() {
	if m.filter.panel == menuView {
		m.clearFilter()
	}

	m.closePopup()
}

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

	return m.pressItem(mn)
}

func (m *Model) pressItem(mn *menu) tea.Cmd {
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

// toggleMenuItem is a tick, or in a plain menu a press.
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

// menuItemKey is guarded on the session's remote.
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

func (m *Model) menuOptionsMap(mn *menu) map[string]string {
	options := map[string]string{"esc": m.tr.Close, "↑ ↓": m.tr.Navigate, "enter": m.tr.Execute}

	if mn.daemon {
		options["[ ]"] = m.tr.SwitchList
		options["/"] = m.tr.LcFilter
	}

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

// optionsBindings are the view's, a gap, the globals.
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
