package gui

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getSnapshotsPanel() *panels.SideListPanel[*commands.Snapshot] {
	return &panels.SideListPanel[*commands.Snapshot]{
		ContextState: &panels.ContextState[*commands.Snapshot]{
			GetMainTabs: func() []panels.MainTab[*commands.Snapshot] {
				return []panels.MainTab[*commands.Snapshot]{
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderSnapshotConfig,
					},
				}
			},
			GetItemContextCacheKey: func(snapshot *commands.Snapshot) string {
				return "snapshots-" + snapshot.Key()
			},
		},
		ListPanel: panels.ListPanel[*commands.Snapshot]{
			List: panels.NewFilteredList[*commands.Snapshot](),
			View: gui.Views.Snapshots,
		},
		NoItemsMessage: gui.Tr.NoSnapshots,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Snapshot, b *commands.Snapshot) bool {
			// Newest first: a rollback almost always means the last one.
			return a.Snapshot.CreatedAt.After(b.Snapshot.CreatedAt)
		},
		GetTableCells: presentation.GetSnapshotDisplayStrings,
	}
}

func (gui *Gui) renderSnapshotConfig(snapshot *commands.Snapshot) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.snapshotConfigStr(snapshot) })
}

func (gui *Gui) snapshotConfigStr(snapshot *commands.Snapshot) string {
	padding := 12
	output := ""
	output += utils.WithPadding("Instance: ", padding) + snapshot.InstanceName + "\n"
	output += utils.WithPadding("Name: ", padding) + snapshot.Name + "\n"
	output += utils.WithPadding("Taken at: ", padding) + snapshot.Snapshot.CreatedAt.String() + "\n"
	output += utils.WithPadding("Stateful: ", padding) + fmt.Sprint(snapshot.Snapshot.Stateful) + "\n"

	data, err := utils.MarshalIntoYaml(snapshot.Snapshot)
	if err != nil {
		return fmt.Sprintf("Error marshalling snapshot details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

// refreshSnapshots reloads the panel for whichever instance is selected.
// Unlike the other panels this one follows another panel's selection, so it
// runs on selection changes as well as on its own poll.
func (gui *Gui) refreshSnapshots() error {
	if gui.Views.Snapshots == nil {
		return nil
	}

	instance, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		gui.Panels.Snapshots.ClearItems()
		gui.setSnapshotsTitle("")

		return gui.Panels.Snapshots.RerenderList()
	}

	gui.setSnapshotsTitle(instance.Name)

	snapshots, err := instance.Snapshots()
	if err != nil {
		return err
	}

	gui.Panels.Snapshots.SetItems(snapshots)

	return gui.Panels.Snapshots.RerenderList()
}

// setSnapshotsTitle names the instance the panel is showing, since the list
// alone gives no clue which instance these snapshots belong to.
func (gui *Gui) setSnapshotsTitle(instanceName string) {
	title := gui.Tr.SnapshotsTitle
	if instanceName != "" {
		title += " (" + instanceName + ")"
	}

	gui.Views.Snapshots.Title = title
}

func (gui *Gui) refreshSnapshotsQuiet() error {
	if err := gui.refreshSnapshots(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

func (gui *Gui) handleSnapshotCreate(g *gocui.Gui, v *gocui.View) error {
	instance, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	return gui.createPromptPanel(fmt.Sprintf(gui.Tr.SnapshotNamePrompt, instance.Name), func(g *gocui.Gui, v *gocui.View) error {
		name := strings.TrimSpace(v.Buffer())
		if name == "" {
			return nil
		}

		return gui.WithWaitingStatus(gui.Tr.SnapshottingStatus, func() error {
			if err := instance.CreateSnapshot(name); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshSnapshots()
		})
	})
}

func (gui *Gui) handleSnapshotRestore(g *gocui.Gui, v *gocui.View) error {
	snapshot, err := gui.Panels.Snapshots.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.RestoreSnapshot, snapshot.InstanceName, snapshot.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RestoringStatus, func() error {
			if err := snapshot.Restore(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshInstances()
		})
	}, nil)
}

func (gui *Gui) handleSnapshotDelete(g *gocui.Gui, v *gocui.View) error {
	snapshot, err := gui.Panels.Snapshots.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteSnapshot, snapshot.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := snapshot.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshSnapshots()
		})
	}, nil)
}
