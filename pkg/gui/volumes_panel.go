package gui

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getVolumesPanel() *panels.SideListPanel[*commands.Volume] {
	return &panels.SideListPanel[*commands.Volume]{
		ContextState: &panels.ContextState[*commands.Volume]{
			GetMainTabs: func() []panels.MainTab[*commands.Volume] {
				return []panels.MainTab[*commands.Volume]{
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderVolumeConfig,
					},
				}
			},
			GetItemContextCacheKey: func(volume *commands.Volume) string {
				// Not the usage, which changes all the time and would redraw
				// the tab with every poll.
				return "volumes-" + volume.Key() + "-" + utils.Fingerprint(volume.Volume)
			},
		},
		ListPanel: panels.ListPanel[*commands.Volume]{
			List: panels.NewFilteredList[*commands.Volume](),
			View: gui.Views.Volumes,
		},
		NoItemsMessage: gui.Tr.NoVolumes,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Volume, b *commands.Volume) bool {
			return sortVolumes(a, b)
		},
		SameItem: func(a, b *commands.Volume) bool {
			return a.Key() == b.Key()
		},
		// The snapshots panel follows a custom volume the way it follows an
		// instance; the other volumes' snapshots are their instance's.
		OnSelect: func(volume *commands.Volume) error {
			if !volume.IsCustom() {
				return nil
			}

			return gui.refreshSnapshotsForVolume(volume)
		},
		GetTableCells: func(item *commands.Volume) []string {
			return presentation.GetVolumeDisplayStrings(item, gui.State.SpansProjects.Volumes)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index: projectColumns(gui.State.SpansProjects.Volumes), MinWidth: presentation.MinVolumeNameWidth,
			}}
		},
	}
}

// sortVolumes groups by pool, then puts the volumes someone created ahead of
// the ones Incus manages for instances and images.
func sortVolumes(a *commands.Volume, b *commands.Volume) bool {
	if a.Pool != b.Pool {
		return a.Pool < b.Pool
	}

	if a.IsCustom() != b.IsCustom() {
		return a.IsCustom()
	}

	if a.Volume.Type != b.Volume.Type {
		return a.Volume.Type < b.Volume.Type
	}

	return a.Name < b.Name
}

func (gui *Gui) renderVolumeConfig(volume *commands.Volume) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.volumeConfigStr(volume) })
}

func (gui *Gui) volumeConfigStr(volume *commands.Volume) string {
	padding := identityPadding
	output := gui.locationStr(gui.sessionLocation(volume.Volume.Project))
	size := presentation.DisplayVolumeUsage(volume)
	if size == "" {
		size = gui.Tr.VolumeSizeUnknown
	}

	usedBy := strings.Join(volume.Users(), ", ")
	if usedBy == "" {
		usedBy = gui.Tr.UsedByNothing
	}

	output += utils.WithPadding("Name: ", padding) + volume.Name + "\n"
	output += utils.WithPadding("Pool: ", padding) + volume.Pool + " (" + volume.PoolDriver + ")\n"
	if space := presentation.DisplayPoolSpace(volume); space != "" {
		output += utils.WithPadding("Pool space: ", padding) + space + "\n"
	}
	output += utils.WithPadding("Type: ", padding) + volume.Volume.Type + "\n"
	output += utils.WithPadding("Content type: ", padding) + volume.Volume.ContentType + "\n"
	output += utils.WithPadding("Size: ", padding) + size + "\n"
	output += utils.WithPadding("Used by: ", padding) + usedBy + "\n"

	data, err := utils.MarshalIntoYaml(volume.Volume)
	if err != nil {
		return fmt.Sprintf("Error marshalling volume details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) fetchVolumes() (func() error, error) {
	ticket := gui.refreshes.volumes.issue()

	volumes, err := gui.IncusCommand.GetVolumes()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.volumes.admit(ticket) {
			return nil
		}

		gui.State.SpansProjects.Volumes = spansMultipleProjects(
			lo.Map(volumes, func(volume *commands.Volume, _ int) string { return volume.Volume.Project }))

		gui.Panels.Volumes.SetItems(volumes)

		if err := gui.Panels.Volumes.RerenderList(); err != nil {
			return err
		}

		// A followed volume's snapshots come with the volumes.
		if _, ok := gui.snapshotsVolume(); ok {
			return gui.renderSnapshots()
		}

		return nil
	}, nil
}

func (gui *Gui) refreshVolumes() error {
	return gui.refresh(nil, gui.fetchVolumes)
}

func (gui *Gui) refreshVolumesQuiet() error {
	if err := gui.refreshVolumes(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

func (gui *Gui) showVolumeUsers(volume *commands.Volume) error {
	return gui.showUsers(volume.Name, volume.IsUsedBy)
}

func (gui *Gui) volumeEdit(volume *commands.Volume) error {
	return gui.editInIncus(volume.Volume.Project, []fetch{gui.fetchVolumes},
		"storage", "volume", "edit", volume.Pool, volume.Volume.Type+"/"+volume.Name)
}

func (gui *Gui) volumeDelete(volume *commands.Volume) error {
	if !volume.IsCustom() {
		return gui.createErrorPanel(gui.Tr.CannotDeleteManagedVolume)
	}

	name := gui.qualified(volume.Name, volume.Volume.Project, gui.State.SpansProjects.Volumes)
	if volume.UsedByCount() > 0 {
		return gui.refuseInUse(fmt.Sprintf(gui.Tr.VolumeNamed, name), volume.UsedByCount())
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteVolume, name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := volume.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshVolumes()
		})
	}, nil)
}
