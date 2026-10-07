package gui

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

// handleCommandPalette opens every action in one menu, filtered as you type:
// the focused panel's and the global ones first, then the other panels',
// which focus their panel before acting on its selection. Typing also lists
// what the panels list, to go to.
func (gui *Gui) handleCommandPalette(g *gocui.Gui, v *gocui.View) error {
	if gui.isPopupPanel(v.Name()) || v == gui.Views.Filter {
		return nil
	}

	return gui.openPalette(g, v, "")
}

// openPalette opens the palette from v; scoped to item, it holds only the
// actions of v's panel, on item, which v's panel has selected.
func (gui *Gui) openPalette(g *gocui.Gui, v *gocui.View, item string) error {
	// A panel's own filter would otherwise be left applied with no way to
	// clear it, the palette taking the filter prompt over.
	if gui.State.Filter.panel != nil {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	// Before the menu opens, so it opens at the palette's width.
	gui.State.Filter.active = true
	gui.State.Filter.panel = gui.Panels.Menu

	title, hint := gui.Tr.CommandPaletteTitle, gui.Tr.CommandPaletteHint
	if item != "" {
		title, hint = fmt.Sprintf("%s: %s", title, item), gui.Tr.CommandPaletteItemHint
	}

	if err := gui.Menu(CreateMenuOptions{
		Title:      title,
		Subtitle:   hint,
		Items:      gui.paletteItems(g, v, item != ""),
		HideCancel: true,
	}); err != nil {
		return err
	}

	if err := gui.setViewContent(gui.Views.FilterPrefix, gui.filterPrompt()); err != nil {
		return err
	}

	return gui.switchFocus(gui.Views.Filter)
}

func (gui *Gui) paletteOpen() bool {
	return gui.State.Filter.panel == gui.Panels.Menu
}

// paletteTab opens the actions of the palette's selected item.
func (gui *Gui) paletteTab() error {
	if !gui.paletteOpen() {
		return gui.cycleSidePanel(1)()
	}

	item, err := gui.Panels.Menu.GetSelectedItem()
	if err != nil || item.OnTab == nil {
		return nil
	}

	if err := gui.handleMenuClose(); err != nil {
		return err
	}

	return item.OnTab()
}

// paletteItems is a row per action: its key, the panel it belongs to, what
// it does and what it would do it to - the panel's selection, on its remote.
// Unless scoped to v's panel, a row per item the panels list follows.
func (gui *Gui) paletteItems(g *gocui.Gui, v *gocui.View, scoped bool) []*types.MenuItem {
	scopes := map[string]string{"": gui.Tr.CommandPaletteGlobal, "main": gui.Tr.MainTitle}
	for _, def := range gui.visibleSidePanelDefs() {
		scopes[def.name] = def.title
	}
	scopes["services"] = gui.Tr.ServicesTitle

	sources := gui.paletteSources()
	sideView := gui.currentSideViewName()

	var items []*types.MenuItem
	seen := map[[2]string]bool{}
	// A key shown dimmed is that panel's, not one to press here.
	add := func(binding *Binding, view string, here bool, run func() error) {
		key := [2]string{binding.ViewName, binding.Description}
		if binding.Description == "" || binding.GetKey() == "" || binding.Key == gocui.KeyCtrlP || seen[key] {
			return
		}

		if view == "main" {
			view = sideView
		}

		target := ""
		var stillSelected func() bool
		if source, ok := sources[view]; ok && binding.ViewName != "" {
			var selected bool
			if target, stillSelected, selected = source.selected(); !selected && !binding.NoSelection {
				return
			}
		}
		// A refresh can move the cursor off the row the palette named.
		if stillSelected != nil && !binding.NoSelection {
			named, act := target, run
			run = func() error {
				if !stillSelected() {
					return gui.createErrorPanel(fmt.Sprintf(gui.Tr.PaletteSelectionGone, named))
				}

				return act()
			}
		}
		// Scoped, the title names the target.
		if scoped {
			target = ""
		}
		seen[key] = true

		if binding.Mutates && gui.isReadOnly(gui.actionRemote(view)) {
			target += " " + utils.ColoredString(gui.Tr.ReadOnly, color.FgYellow)
		}

		keyLabel := binding.GetKey()
		if !here {
			keyLabel = utils.ColoredString(keyLabel, color.FgHiBlack)
		}

		items = append(items, &types.MenuItem{
			LabelColumns: []string{keyLabel, scopes[binding.ViewName], binding.Description, target},
			FilterText:   binding.Description + " " + scopes[binding.ViewName],
			OnPress:      run,
		})
	}

	focused := map[string]bool{v.Name(): true}
	for _, binding := range gui.getBindings(v) {
		if scoped && binding.ViewName != v.Name() {
			continue
		}

		focused[binding.ViewName] = true
		add(binding, v.Name(), true, func() error { return binding.Handler(g, v) })
	}

	if scoped {
		return items
	}

	bindings := gui.GetInitialKeybindings()
	for _, def := range gui.visibleSidePanelDefs() {
		if focused[def.name] {
			continue
		}

		view := *def.viewPtr
		for _, binding := range bindings {
			if binding.ViewName != def.name {
				continue
			}

			add(binding, def.name, false, func() error {
				if err := gui.switchFocus(view); err != nil {
					return err
				}

				return binding.Handler(g, view)
			})
		}
	}

	for _, def := range gui.visibleSidePanelDefs() {
		source, ok := sources[def.name]
		if !ok {
			continue
		}

		view := *def.viewPtr
		for _, entry := range source.items() {
			goTo := func() bool {
				if !entry.goTo() {
					_ = gui.createErrorPanel(fmt.Sprintf(gui.Tr.PaletteItemGone, entry.name))
					return false
				}

				return gui.switchFocus(view) == nil
			}

			items = append(items, &types.MenuItem{
				LabelColumns:      []string{"", scopes[def.name], utils.ColoredString(entry.name, color.FgCyan), entry.detail},
				FilterText:        entry.name + " " + scopes[def.name],
				HideUntilFiltered: true,
				OnPress: func() error {
					goTo()
					return nil
				},
				OnTab: func() error {
					if !goTo() {
						return nil
					}

					return gui.openPalette(g, view, entry.name)
				},
			})
		}
	}

	return items
}

// paletteSource is a panel as the palette sees it: its selection, named and
// checkable later, and every item it lists, to go to.
type paletteSource struct {
	selected func() (name string, stillSelected func() bool, ok bool)
	items    func() []paletteEntry
}

type paletteEntry struct {
	name, detail string
	goTo         func() bool
}

func (gui *Gui) paletteSources() map[string]paletteSource {
	return map[string]paletteSource{
		"stacks": sourceOf(gui.Panels.Stacks, func(stack *commands.ComposeStack) string {
			return gui.onRemote(stack.Name, stack.Remote)
		}, nil),
		"services": sourceOf(gui.Panels.Services, func(row *commands.ServiceRow) string {
			name := row.Service.Name
			if row.Instance != nil {
				name = row.Instance.Name
			}

			return gui.onRemote(name, gui.actionRemote("services"))
		}, nil),
		"instances": sourceOf(gui.Panels.Instances, func(instance *commands.Instance) string {
			return gui.onRemote(instance.Name, instance.Remote)
		}, func(instance *commands.Instance) string { return instance.Project }),
		"snapshots": sourceOf(gui.Panels.Snapshots, func(snapshot *commands.Snapshot) string {
			return gui.onRemote(snapshot.Owner+"/"+snapshot.Name, gui.actionRemote("snapshots"))
		}, nil),
		"backups": sourceOf(gui.Panels.Backups, func(backup *commands.ComposeBackup) string {
			return gui.onRemote(gui.backupLabel(backup), gui.actionRemote("backups"))
		}, nil),
		"images": sourceOf(gui.Panels.Images, (*commands.Image).Label, nil),
		"volumes": sourceOf(gui.Panels.Volumes, func(volume *commands.Volume) string { return volume.Name },
			func(volume *commands.Volume) string { return volume.Pool }),
		"networks": sourceOf(gui.Panels.Networks, func(network *commands.Network) string { return network.Name }, nil),
		"profiles": sourceOf(gui.Panels.Profiles, func(profile *commands.Profile) string { return profile.Name }, nil),
	}
}

// sourceOf is panel's paletteSource, its items named by name and detail,
// which can be nil.
func sourceOf[T comparable](panel *panels.SideListPanel[T], name func(T) string, detail func(T) string) paletteSource {
	return paletteSource{
		selected: func() (string, func() bool, bool) {
			if panel == nil {
				return "", nil, false
			}

			item, err := panel.GetSelectedItem()
			if err != nil {
				return "", nil, false
			}

			return name(item), func() bool { return panel.IsSelected(item) }, true
		},
		items: func() []paletteEntry {
			if panel == nil {
				return nil
			}

			var entries []paletteEntry
			for _, item := range panel.List.GetItems() {
				entry := paletteEntry{name: name(item), goTo: func() bool { return panel.Select(item) }}
				if detail != nil {
					entry.detail = detail(item)
				}
				entries = append(entries, entry)
			}

			return entries
		},
	}
}
