package gui

import (
	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

func (gui *Gui) getBindings(v *gocui.View) []*Binding {
	var bindingsGlobal, bindingsPanel []*Binding

	bindings := gui.GetInitialKeybindings()

	for _, binding := range bindings {
		if binding.listed() {
			switch binding.ViewName {
			case "":
				bindingsGlobal = append(bindingsGlobal, binding)
			case v.Name():
				bindingsPanel = append(bindingsPanel, binding)
			}
		}
	}

	if v.ParentView != nil {
	L:
		for _, binding := range bindings {
			if binding.listed() {
				if binding.ViewName == v.ParentView.Name() {
					for _, ownBinding := range bindingsPanel {
						if binding.Key != nil && ownBinding.GetKey() == binding.GetKey() {
							continue L
						}
					}
					bindingsPanel = append(bindingsPanel, binding)
				}
			}
		}
	}

	bindingsPanel = append(bindingsPanel, &Binding{})
	return append(bindingsPanel, bindingsGlobal...)
}

func (gui *Gui) handleCreateOptionsMenu(g *gocui.Gui, v *gocui.View) error {
	if gui.isPopupPanel(v.Name()) {
		return nil
	}

	menuItems := lo.Map(gui.getBindings(v), func(binding *Binding, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: []string{binding.GetKey(), binding.Description},
			OnPress: func() error {
				if binding.Handler == nil {
					return nil
				}

				return binding.Handler(g, v)
			},
		}
	})

	return gui.Menu(CreateMenuOptions{
		Title:      gui.Tr.MenuTitle,
		Items:      menuItems,
		HideCancel: true,
	})
}
