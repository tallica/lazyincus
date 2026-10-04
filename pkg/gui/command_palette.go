package gui

import (
	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

// handleCommandPalette opens every action in one menu, filtered as you type:
// the focused panel's and the global ones first, then the other panels',
// which focus their panel before acting on its selection.
func (gui *Gui) handleCommandPalette(g *gocui.Gui, v *gocui.View) error {
	if gui.isPopupPanel(v.Name()) || v == gui.Views.Filter {
		return nil
	}

	// Before the menu opens, so it opens at the palette's width.
	gui.State.Filter.active = true
	gui.State.Filter.panel = gui.Panels.Menu

	if err := gui.Menu(CreateMenuOptions{
		Title:      gui.Tr.CommandPaletteTitle,
		Items:      gui.paletteItems(g, v),
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

// paletteItems is a row per action: its key, the panel it belongs to, what
// it does and what it would do it to - the panel's selection, on its remote.
func (gui *Gui) paletteItems(g *gocui.Gui, v *gocui.View) []*types.MenuItem {
	scopes := map[string]string{"": gui.Tr.CommandPaletteGlobal, "main": gui.Tr.MainTitle}
	for _, def := range gui.visibleSidePanelDefs() {
		scopes[def.name] = def.title
	}
	scopes["services"] = gui.Tr.ServicesTitle

	targets := gui.paletteTargets()
	sideView := gui.currentSideViewName()

	var items []*types.MenuItem
	seen := map[[2]string]bool{}
	// A key shown dimmed is that panel's, not one to press here.
	add := func(binding *Binding, view string, here bool, run func() error) {
		key := [2]string{binding.ViewName, binding.Description}
		if binding.Description == "" || binding.GetKey() == "" || seen[key] {
			return
		}

		if view == "main" {
			view = sideView
		}

		target := ""
		if label, ok := targets[view]; ok && binding.ViewName != "" {
			var selected bool
			if target, selected = label(); !selected && !binding.NoSelection {
				return
			}
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
			FilterText:   binding.Description + " " + scopes[binding.ViewName] + " " + utils.Decolorise(target),
			OnPress:      run,
		})
	}

	focused := map[string]bool{v.Name(): true}
	for _, binding := range gui.getBindings(v) {
		focused[binding.ViewName] = true
		add(binding, v.Name(), true, func() error { return binding.Handler(g, v) })
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

	return items
}

// paletteTargets names each panel's selection, with its remote when that
// isn't the session's.
func (gui *Gui) paletteTargets() map[string]func() (string, bool) {
	return map[string]func() (string, bool){
		"stacks": selectedLabel(gui.Panels.Stacks, func(stack *commands.ComposeStack) string {
			return gui.onRemote(stack.Name, stack.Remote)
		}),
		"services": selectedLabel(gui.Panels.Services, func(row *commands.ServiceRow) string {
			name := row.Service.Name
			if row.Instance != nil {
				name = row.Instance.Name
			}

			return gui.onRemote(name, gui.actionRemote("services"))
		}),
		"instances": selectedLabel(gui.Panels.Instances, func(instance *commands.Instance) string {
			return gui.onRemote(instance.Name, instance.Remote)
		}),
		"snapshots": selectedLabel(gui.Panels.Snapshots, func(snapshot *commands.Snapshot) string {
			return gui.onRemote(snapshot.Owner+"/"+snapshot.Name, gui.actionRemote("snapshots"))
		}),
		"images":   selectedLabel(gui.Panels.Images, (*commands.Image).Label),
		"volumes":  selectedLabel(gui.Panels.Volumes, func(volume *commands.Volume) string { return volume.Name }),
		"networks": selectedLabel(gui.Panels.Networks, func(network *commands.Network) string { return network.Name }),
		"profiles": selectedLabel(gui.Panels.Profiles, func(profile *commands.Profile) string { return profile.Name }),
	}
}

// selectedLabel is the panel's selection named by label, and whether it has
// one.
func selectedLabel[T comparable](panel *panels.SideListPanel[T], label func(T) string) func() (string, bool) {
	return func() (string, bool) {
		if panel == nil {
			return "", false
		}

		item, err := panel.GetSelectedItem()
		if err != nil {
			return "", false
		}

		return label(item), true
	}
}
