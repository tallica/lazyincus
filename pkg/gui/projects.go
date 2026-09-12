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

	allProjects := &types.MenuItem{
		LabelColumns: []string{marker(gui.IncusCommand.IsAllProjects()), gui.Tr.AllProjects},
		OnPress:      gui.switchToAllProjects,
	}

	menuItems := lo.Map(names, func(name string, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: []string{marker(name == current), name},
			OnPress: func() error {
				return gui.switchToProject(name)
			},
		}
	})

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ProjectsTitle,
		Items: append([]*types.MenuItem{allProjects}, menuItems...),
	})
}

func marker(selected bool) string {
	if selected {
		return "*"
	}

	return " "
}

// switchToAllProjects lists every project's instances, images, volumes and
// networks at once. Actions still run against the project each item came
// from, so this is a wider view rather than a different mode.
func (gui *Gui) switchToAllProjects() error {
	if gui.IncusCommand.IsAllProjects() {
		return nil
	}

	gui.IncusCommand.UseAllProjects()

	return gui.reloadAfterProjectChange()
}

func (gui *Gui) switchToProject(name string) error {
	if name == gui.IncusCommand.ProjectName() {
		return nil
	}

	gui.IncusCommand.UseProject(name)

	return gui.reloadAfterProjectChange()
}

func (gui *Gui) reloadAfterProjectChange() error {
	// Everything the panels hold belongs to the scope we just left. Dropping
	// it also stops the next refresh matching new items against stale ones by
	// name.
	for _, panel := range gui.allSidePanels() {
		panel.ClearItems()
	}

	gui.Panels.Instances.SetSelectedLineIdx(0)

	for _, refresh := range []func() error{
		gui.refreshInstances,
		gui.refreshImages,
		gui.refreshVolumes,
		gui.refreshNetworks,
	} {
		if err := refresh(); err != nil {
			return gui.createErrorPanel(err.Error())
		}
	}

	return gui.renderString(gui.g, "information", gui.getInformationContent())
}
