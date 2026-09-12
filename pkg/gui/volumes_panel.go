package gui

import (
	"fmt"
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
				return "volumes-" + volume.Key()
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
		GetTableCells: func(item *commands.Volume) []string {
			return presentation.GetVolumeDisplayStrings(item, gui.State.SpansProjects.Volumes)
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
	padding := 14
	output := ""
	output += utils.WithPadding("Name: ", padding) + volume.Name + "\n"
	output += utils.WithPadding("Pool: ", padding) + volume.Pool + "\n"
	output += utils.WithPadding("Type: ", padding) + volume.Volume.Type + "\n"
	output += utils.WithPadding("Content type: ", padding) + volume.Volume.ContentType + "\n"
	output += utils.WithPadding("Used by: ", padding) + fmt.Sprint(volume.UsedByCount()) + "\n"

	data, err := utils.MarshalIntoYaml(volume.Volume)
	if err != nil {
		return fmt.Sprintf("Error marshalling volume details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) refreshVolumes() error {
	if gui.Views.Volumes == nil {
		return nil
	}

	volumes, err := gui.IncusCommand.GetVolumes(gui.Panels.Volumes.List.GetAllItems())
	if err != nil {
		return err
	}

	gui.State.SpansProjects.Volumes = spansMultipleProjects(
		lo.Map(volumes, func(volume *commands.Volume, _ int) string { return volume.Volume.Project }))

	gui.Panels.Volumes.SetItems(volumes)

	return gui.Panels.Volumes.RerenderList()
}

func (gui *Gui) refreshVolumesQuiet() error {
	if err := gui.refreshVolumes(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

func (gui *Gui) handleVolumeDelete(g *gocui.Gui, v *gocui.View) error {
	volume, err := gui.Panels.Volumes.GetSelectedItem()
	if err != nil {
		return nil
	}

	if !volume.IsCustom() {
		return gui.createErrorPanel(gui.Tr.CannotDeleteManagedVolume)
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteVolume, volume.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := volume.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshVolumes()
		})
	}, nil)
}
