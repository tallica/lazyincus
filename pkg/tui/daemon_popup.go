package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

type daemonTab int

const (
	daemonOperations daemonTab = iota
	daemonWarnings
)

// daemonState is pkg/gui's DaemonTab, DaemonOpened, OperationsShown and SpansProjects.Operations.
type daemonState struct {
	tab           daemonTab
	opened        bool
	shown         operationsShown
	spansProjects bool
	// filtering is the filter prompt having the focus over the popup.
	filtering bool
}

type operationsShown int

const (
	operationsAll operationsShown = iota
	operationsFailed
	operationsSucceeded
)

func (shown operationsShown) has(operation *commands.Operation) bool {
	switch shown {
	case operationsFailed:
		return operation.Operation.StatusCode == api.Failure
	case operationsSucceeded:
		return operation.Operation.StatusCode == api.Success
	default:
		return true
	}
}

func (m *Model) daemonPopup() *popup {
	for _, p := range m.popups {
		if p.menu != nil && p.menu.daemon {
			return p
		}
	}

	return nil
}

func (m *Model) daemonOpen(tab daemonTab) bool {
	return m.daemonPopup() != nil && m.daemon.tab == tab
}

// openDaemonKey is pkg/gui's handleOpenDaemon.
func (m *Model) openDaemonKey() tea.Cmd {
	tab := m.daemon.tab
	if !m.daemon.opened && m.newWarnings > 0 {
		tab = daemonWarnings
	}

	m.openDaemon(tab, "")

	return refresh(m.fetchOperations(), m.fetchWarnings())
}

func (m *Model) openDaemon(tab daemonTab, selected string) {
	m.daemon.tab, m.daemon.opened = tab, true

	m.openMenu(menuOptions{
		subtitle:    m.daemonHint(tab),
		items:       m.daemonItems(tab),
		hideCancel:  true,
		tabs:        m.daemonTabs(),
		tabIndex:    int(tab),
		wide:        true,
		flexColumns: m.daemonFlexColumns(tab),
		daemon:      true,
	})

	if tab == daemonOperations {
		m.seeOperations()
	}

	mn := m.topMenu()
	mn.list.selected = max(daemonRow(mn, selected), 0)
}

// switchDaemonTab is pkg/gui's handleDaemonTab.
func (m *Model) switchDaemonTab(step int) {
	if mn := m.topMenu(); mn == nil || !mn.daemon {
		return
	}

	tabs := 2
	m.openDaemon(daemonTab((int(m.daemon.tab)+step+tabs)%tabs), "")
}

// openDaemonFilter is pkg/gui's handleDaemonFilter.
func (m *Model) openDaemonFilter() {
	if mn := m.topMenu(); mn == nil || !mn.daemon {
		return
	}

	m.filter.active = true
	m.filter.panel = menuView
	m.daemon.filtering = true
}

// cycleOperationsShown is pkg/gui's handleOperationsShown.
func (m *Model) cycleOperationsShown() {
	if mn := m.topMenu(); mn == nil || !mn.daemon || m.daemon.tab != daemonOperations {
		return
	}

	selected := m.daemonSelected(daemonOperations)
	m.daemon.shown = (m.daemon.shown + 1) % (operationsSucceeded + 1)
	m.redrawDaemon(daemonOperations, selected)
}

func (m *Model) operationsShownLabel() string {
	switch m.daemon.shown {
	case operationsFailed:
		return m.tr.OperationsFailedOnly
	case operationsSucceeded:
		return m.tr.OperationsSucceededOnly
	default:
		return ""
	}
}

// daemonTabs names the operations after what they're cut down to.
func (m *Model) daemonTabs() []string {
	operations := m.tr.OperationsTitle
	if shown := m.operationsShownLabel(); shown != "" {
		operations += " (" + shown + ")"
	}

	return []string{operations, m.tr.WarningsTitle}
}

func (m *Model) daemonHint(tab daemonTab) string {
	if tab == daemonOperations {
		return m.tr.OperationsHint
	}

	return m.tr.WarningsHint
}

func (m *Model) daemonItems(tab daemonTab) []*menuItem {
	if tab == daemonOperations {
		return m.operationItems()
	}

	return m.warningItems()
}

func (m *Model) daemonFlexColumns(tab daemonTab) []utils.FlexColumn {
	if tab == daemonOperations {
		return presentation.OperationFlexColumns(m.daemon.spansProjects, presentation.OperationTookWidth(m.operations.list()))
	}

	return nil
}

// operationItems are under way first, being what the list is for, then newest first.
func (m *Model) operationItems() []*menuItem {
	operations := lo.Filter(m.operations.list(), func(operation *commands.Operation, _ int) bool {
		return m.daemon.shown.has(operation)
	})
	slices.SortStableFunc(operations, func(a, b *commands.Operation) int {
		if a.IsFinal() != b.IsFinal() {
			if a.IsFinal() {
				return 1
			}

			return -1
		}

		return b.Operation.CreatedAt.Compare(a.Operation.CreatedAt)
	})

	items := make([]*menuItem, 0, len(operations))
	for _, operation := range operations {
		items = append(items, &menuItem{
			key:          operation.Key(),
			labelColumns: presentation.GetOperationDisplayStrings(operation, m.daemon.spansProjects, m.operations.retried(operation)),
			filterText:   operationLabel(operation) + " " + operation.Project,
			onPress:      func(m *Model) tea.Cmd { return m.showOperation(operation) },
			keys: map[string]menuKey{
				"d": {handler: func(m *Model) tea.Cmd { return m.operationCancel(operation) }, mutates: true},
			},
		})
	}

	if len(items) == 0 {
		empty := m.tr.NoOperations
		if shown := m.operationsShownLabel(); shown != "" {
			empty = fmt.Sprintf(m.tr.NoOperationsShown, shown)
		}

		items = append(items, &menuItem{labelColumns: []string{empty}})
	}

	return items
}

func (m *Model) warningItems() []*menuItem {
	items := make([]*menuItem, 0, len(m.warnings))
	for _, warning := range m.warnings {
		items = append(items, &menuItem{
			key:          warning.Key(),
			labelColumns: presentation.GetWarningDisplayStrings(warning),
			filterText:   warning.Warning.Type + " " + warning.Warning.LastMessage,
			onPress:      func(m *Model) tea.Cmd { return m.showWarning(warning) },
			keys: map[string]menuKey{
				"a": {handler: func(m *Model) tea.Cmd { return m.acknowledgeWarning(warning) }, mutates: true},
				"d": {handler: func(m *Model) tea.Cmd { return m.deleteWarning(warning) }, mutates: true},
			},
		})
	}

	if len(items) == 0 {
		items = append(items, &menuItem{labelColumns: []string{m.tr.NoWarnings}})
	}

	return items
}

func daemonRow(mn *menu, key string) int {
	if key == "" {
		return -1
	}

	return slices.IndexFunc(mn.list.shown, func(item int) bool { return mn.items[item].key == key })
}

// daemonSelected is pkg/gui's: read it before the rows change, for redrawDaemon.
func (m *Model) daemonSelected(tab daemonTab) string {
	p := m.daemonPopup()
	if p == nil || m.daemon.tab != tab {
		return ""
	}

	// On the first row, the cursor follows the newest.
	if tab == daemonOperations && p.menu.list.selected == 0 {
		return ""
	}

	if item := p.menu.selectedItem(); item != nil {
		return item.key
	}

	return ""
}

// redrawDaemon is pkg/gui's: the popup in place, the cursor on the row keyed selected.
func (m *Model) redrawDaemon(tab daemonTab, selected string) {
	p := m.daemonPopup()
	if p == nil || m.daemon.tab != tab {
		return
	}

	mn := p.menu
	was := mn.list.selected
	mn.tabs = m.daemonTabs()
	p.title = strings.Join(mn.tabs, " - ")
	mn.flexColumns = m.daemonFlexColumns(tab)
	mn.items = m.daemonItems(tab)
	padMenuItems(mn.items)
	m.renderMenu(mn)

	index := max(daemonRow(mn, selected), 0)
	mn.list.selected = index

	if tab == daemonOperations {
		// The rows on screen stay put as new ones arrive above them.
		if index != was {
			mn.list.origin = max(mn.list.origin+index-was, 0)
		}

		m.seeOperations()
	}
}

// seeOperations is the operations on screen: their failures are seen.
func (m *Model) seeOperations() {
	m.operationsSeenAt = time.Now()
	m.countOperations()
}

// showOperations is pkg/gui's: selected is the row the popup had before the log changed.
func (m *Model) showOperations(selected string) {
	m.daemon.spansProjects = spansProjects(m.operations.list(), func(operation *commands.Operation) string { return operation.Project })
	m.redrawDaemon(daemonOperations, selected)
	m.countOperations()
}

func operationLabel(operation *commands.Operation) string {
	if target := operation.Target(); target != "" {
		return operation.Operation.Description + " " + target
	}

	return operation.Operation.Description
}
