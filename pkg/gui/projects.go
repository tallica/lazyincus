package gui

import (
	"sort"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// handleSwitchProject re-scopes the instance list to another Incus project.
// Instances outside the remote's configured project are invisible otherwise,
// which hides every incus-compose stack: incus-compose gives each compose
// project an Incus project of its own.
func (gui *Gui) handleSwitchProject(g *gocui.Gui, v *gocui.View) error {
	go func() {
		names, err := gui.IncusCommand.GetProjectNames()

		gui.g.Update(func(*gocui.Gui) error {
			if err != nil {
				return err
			}

			return gui.projectsMenu(names)
		})
	}()

	return nil
}

func (gui *Gui) projectsMenu(names []string) error {
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

	return gui.reloadAfterScopeChange()
}

func (gui *Gui) switchToProject(name string) error {
	if name == gui.IncusCommand.ProjectName() {
		return nil
	}

	gui.IncusCommand.UseProject(name)

	return gui.reloadAfterScopeChange()
}

func (gui *Gui) reloadAfterScopeChange() error {
	// Everything the panels hold belongs to the scope we just left, but for
	// the stacks: they're directories, which the refresh below re-reads.
	// Dropping it also stops the next refresh matching new items against
	// stale ones by name.
	for _, panel := range gui.allSidePanels() {
		if panel != panels.ISideListPanel(gui.Panels.Stacks) {
			panel.Await(gui.Tr.Loading)
		}
	}

	gui.composeInstances.Store(nil)
	gui.composeProject.Store(nil)
	// Or the next instance or volume refresh redraws the old scope's
	// snapshots, a volume by a key the new scope may share.
	gui.State.SnapshotsInstances, gui.State.SnapshotsLabel = nil, ""
	gui.State.SnapshotsVolume = ""
	gui.setSnapshotsTitle("")

	// The blank frame below shows "Loading…" in the main panel; without
	// this, the same instance arriving at the top reads as nothing changed.
	gui.resetMainView()

	gui.Panels.Instances.SetSelectedLineIdx(0)

	// The history is the scope's, as every list is.
	gui.operationsScope.Add(1)
	gui.operations.clear()
	gui.State.OperationsSeenAt = time.Now()

	// A fetch already in flight was asked about the old scope.
	gui.refreshes.invalidateAll()
	gui.rescopeEvents()

	for _, panel := range gui.allSidePanels() {
		if err := panel.RerenderList(); err != nil {
			return err
		}
	}

	if err := gui.renderString(gui.g, "information", gui.getInformationContent()); err != nil {
		return err
	}

	return gui.WithWaitingStatus(gui.Tr.LoadingStatus, func() error {
		// One popup for a remote that has gone, not one per group.
		if errs := gui.refreshAll(); len(errs) > 0 {
			return errs[0]
		}

		return nil
	})
}
