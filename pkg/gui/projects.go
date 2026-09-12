package gui

import (
	"sort"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// handleSwitchProject re-scopes the instance list to another Incus project.
// Instances outside the remote's configured project are invisible otherwise,
// which hides every incus-compose stack: incus-compose gives each compose
// project an Incus project of its own.
func (gui *Gui) handleSwitchProject(g *gocui.Gui, v *gocui.View) error {
	names, err := gui.IncusCommand.GetProjectNames()
	if err != nil {
		return gui.createErrorPanel(err.Error())
	}

	sort.Strings(names)
	current := gui.IncusCommand.ProjectName()

	menuItems := lo.Map(names, func(name string, _ int) *types.MenuItem {
		marker := " "
		if name == current {
			marker = "*"
		}

		return &types.MenuItem{
			LabelColumns: []string{marker, name},
			OnPress: func() error {
				return gui.switchToProject(name)
			},
		}
	})

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ProjectsTitle,
		Items: menuItems,
	})
}

func (gui *Gui) switchToProject(name string) error {
	if name == gui.IncusCommand.ProjectName() {
		return nil
	}

	gui.IncusCommand.UseProject(name)

	// Everything the panels hold belongs to the project we just left. Dropping
	// it also stops the next refresh matching new items against stale ones by
	// name.
	for _, panel := range gui.allSidePanels() {
		panel.ClearItems()
	}

	gui.Panels.Instances.SetSelectedLineIdx(0)

	if err := gui.refreshInstances(); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	return gui.renderString(gui.g, "information", gui.getInformationContent())
}
