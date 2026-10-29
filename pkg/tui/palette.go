package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/utils"
)

// openPaletteKey is pkg/gui's handleCommandPalette.
func (m *Model) openPaletteKey() {
	if len(m.popups) > 0 || m.currentView() == filterView {
		return
	}

	m.openPalette(m.currentView(), "")
}

// openPalette is pkg/gui's; scoped to item, only view's actions on it.
func (m *Model) openPalette(view, item string) {
	// The palette takes the prompt over, from a list's filter or a search.
	if m.filter.active {
		m.clearFilter()
	}

	m.filter.active = true
	m.filter.panel = menuView

	title, hint := m.tr.CommandPaletteTitle, m.tr.CommandPaletteHint
	if item != "" {
		title, hint = fmt.Sprintf("%s: %s", title, item), m.tr.CommandPaletteItemHint
	}

	m.openMenu(menuOptions{title: title, subtitle: hint, items: m.paletteItems(view, item != ""), hideCancel: true})
}

func (m *Model) paletteOpen() bool {
	return m.filter.panel == menuView
}

// paletteTab is pkg/gui's: the selected item's actions, else tab's own.
func (m *Model) paletteTab() tea.Cmd {
	if !m.paletteOpen() {
		m.cycleWindows(1)
		return nil
	}

	item := m.topMenu().selectedItem()
	if item == nil || item.onTab == nil {
		return nil
	}

	m.closeMenu()

	return item.onTab(m)
}

// paletteItems is pkg/gui's: a row per action, then unless scoped per item.
func (m *Model) paletteItems(view string, scoped bool) []*menuItem {
	scopes := map[string]string{"": m.tr.CommandPaletteGlobal, mainView: m.tr.MainTitle}
	for _, p := range m.visiblePanels() {
		scopes[p.name] = p.title()
	}
	scopes["services"] = m.tr.ServicesTitle

	sources := m.paletteSources()
	sideView := m.currentSideView()

	var items []*menuItem
	seen := map[[2]string]bool{}
	// A key shown dimmed is that panel's, not one to press here.
	add := func(b *binding, view string, here bool, run func(*Model) tea.Cmd) {
		key := [2]string{b.view, b.description}
		if !b.listed() || b.key == "ctrl+p" || seen[key] {
			return
		}

		if view == mainView {
			view = sideView
		}

		target := ""
		var stillSelected func() bool
		if source, ok := sources[view]; ok && b.view != "" {
			var selected bool
			if target, stillSelected, selected = source.selected(); !selected && !b.noSelection {
				return
			}
		}
		// A refresh can move the cursor off the row the palette named.
		if stillSelected != nil && !b.noSelection {
			named, act := target, run
			run = func(m *Model) tea.Cmd {
				if !stillSelected() {
					m.openError(fmt.Sprintf(m.tr.PaletteSelectionGone, named))
					return nil
				}

				return act(m)
			}
		}
		if scoped {
			target = ""
		}
		seen[key] = true

		if b.mutates && m.isReadOnly(m.actionRemote(view)) {
			target += " " + utils.ColoredString(m.tr.ReadOnly, color.FgYellow)
		}

		label := b.label()
		if !here {
			label = utils.ColoredString(label, color.FgHiBlack)
		}

		items = append(items, &menuItem{
			labelColumns: []string{label, scopes[b.view], b.description, target},
			filterText:   b.description + " " + scopes[b.view],
			onPress:      run,
		})
	}

	focused := map[string]bool{view: true}
	for _, b := range m.optionsBindings(view) {
		if scoped && b.view != view {
			continue
		}

		focused[b.view] = true
		add(b, view, true, b.handler)
	}

	if scoped {
		return items
	}

	bindings := m.bindings()
	for _, p := range m.visiblePanels() {
		if focused[p.name] {
			continue
		}

		for _, b := range bindings {
			if b.view != p.name {
				continue
			}

			add(b, p.name, false, func(m *Model) tea.Cmd {
				m.focusView(p.name)
				return b.handler(m)
			})
		}
	}

	for _, p := range m.visiblePanels() {
		source, ok := sources[p.name]
		if !ok {
			continue
		}

		for _, entry := range source.items() {
			goTo := func(m *Model) bool {
				if !entry.goTo() {
					m.openError(fmt.Sprintf(m.tr.PaletteItemGone, entry.name))
					return false
				}

				m.focusView(p.name)
				return true
			}

			items = append(items, &menuItem{
				labelColumns:      []string{"", scopes[p.name], utils.ColoredString(entry.name, color.FgCyan), entry.detail},
				filterText:        entry.name + " " + scopes[p.name],
				hideUntilFiltered: true,
				onPress: func(m *Model) tea.Cmd {
					goTo(m)
					return nil
				},
				onTab: func(m *Model) tea.Cmd {
					if goTo(m) {
						m.openPalette(p.name, entry.name)
					}
					return nil
				},
			})
		}
	}

	return items
}

// paletteSource is pkg/gui's: a panel's selection and its items.
type paletteSource struct {
	selected func() (name string, stillSelected func() bool, ok bool)
	items    func() []paletteEntry
}

type paletteEntry struct {
	name, detail string
	goTo         func() bool
}

// paletteSources is pkg/gui's, for the panels ported so far.
func (m *Model) paletteSources() map[string]paletteSource {
	return map[string]paletteSource{
		"instances": m.sourceOf(m.panelNamed("instances"),
			func(item int) string { return m.instances[item].Key() },
			func(item int) string { return m.onRemote(m.instances[item].Name, m.instances[item].Remote) },
			func(item int) string { return m.instances[item].Project }),
		"images": m.sourceOf(m.panelNamed("images"),
			func(item int) string { return m.images[item].Key() },
			func(item int) string { return m.images[item].Label() }, nil),
		"volumes": m.sourceOf(m.panelNamed("volumes"),
			func(item int) string { return m.volumes[item].Key() },
			func(item int) string { return m.volumes[item].Name },
			func(item int) string { return m.volumes[item].Pool }),
		"networks": m.sourceOf(m.panelNamed("networks"),
			func(item int) string { return m.networks[item].Key() },
			func(item int) string { return m.networks[item].Name }, nil),
		"snapshots": m.sourceOf(m.panelNamed("snapshots"),
			func(item int) string { return m.snapshots[item].Key() },
			func(item int) string {
				return m.onRemote(m.snapshots[item].Owner+"/"+m.snapshots[item].Name, m.actionRemote("snapshots"))
			}, nil),
	}
}

// sourceOf is pkg/gui's, items told apart by key; detail can be nil.
func (m *Model) sourceOf(p *panel, key, name, detail func(item int) string) paletteSource {
	rowOf := func(k string) int {
		return slices.IndexFunc(p.shown, func(item int) bool { return key(item) == k })
	}

	return paletteSource{
		selected: func() (string, func() bool, bool) {
			item := p.item()
			if item < 0 {
				return "", nil, false
			}

			k := key(item)
			still := func() bool {
				item := p.item()
				return item >= 0 && key(item) == k
			}

			return name(item), still, true
		},
		items: func() []paletteEntry {
			entries := make([]paletteEntry, 0, p.count())
			for _, item := range p.shown {
				k := key(item)
				entry := paletteEntry{name: name(item), goTo: func() bool {
					row := rowOf(k)
					if row < 0 {
						return false
					}
					p.selectRow(row, m.listRows(p))
					return true
				}}
				if detail != nil {
					entry.detail = detail(item)
				}
				entries = append(entries, entry)
			}

			return entries
		},
	}
}

// onRemote is pkg/gui's: what, naming its remote unless it's the session's.
func (m *Model) onRemote(what, remote string) string {
	if remote == "" || remote == m.incus.RemoteName() {
		return what
	}

	return fmt.Sprintf(m.tr.OnRemote, what, remote)
}
