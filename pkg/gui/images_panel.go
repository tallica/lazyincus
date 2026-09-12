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

func (gui *Gui) getImagesPanel() *panels.SideListPanel[*commands.Image] {
	return &panels.SideListPanel[*commands.Image]{
		ContextState: &panels.ContextState[*commands.Image]{
			GetMainTabs: func() []panels.MainTab[*commands.Image] {
				return []panels.MainTab[*commands.Image]{
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderImageConfig,
					},
				}
			},
			GetItemContextCacheKey: func(image *commands.Image) string {
				return "images-" + image.Fingerprint
			},
		},
		ListPanel: panels.ListPanel[*commands.Image]{
			List: panels.NewFilteredList[*commands.Image](),
			View: gui.Views.Images,
		},
		NoItemsMessage: gui.Tr.NoImages,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Image, b *commands.Image) bool {
			return sortImages(a, b)
		},
		GetTableCells: presentation.GetImageDisplayStrings,
	}
}

// sortImages orders by the label the panel actually shows, so the list reads
// in the order it's sorted; the fingerprint breaks ties between images
// sharing a description.
func sortImages(a *commands.Image, b *commands.Image) bool {
	left, right := strings.ToLower(a.Label()), strings.ToLower(b.Label())
	if left != right {
		return left < right
	}

	return a.Fingerprint < b.Fingerprint
}

func (gui *Gui) renderImageConfig(image *commands.Image) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.imageConfigStr(image) })
}

func (gui *Gui) imageConfigStr(image *commands.Image) string {
	padding := 14
	output := ""
	output += utils.WithPadding("Alias: ", padding) + image.Alias() + "\n"
	output += utils.WithPadding("Fingerprint: ", padding) + image.Fingerprint + "\n"
	output += utils.WithPadding("Type: ", padding) + image.Image.Type + "\n"
	output += utils.WithPadding("Architecture: ", padding) + image.Image.Architecture + "\n"
	output += utils.WithPadding("Uploaded: ", padding) + image.Image.UploadedAt.String() + "\n"

	data, err := utils.MarshalIntoYaml(image.Image)
	if err != nil {
		return fmt.Sprintf("Error marshalling image details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) refreshImages() error {
	if gui.Views.Images == nil {
		return nil
	}

	images, err := gui.IncusCommand.GetImages(gui.Panels.Images.List.GetAllItems())
	if err != nil {
		return err
	}

	gui.Panels.Images.SetItems(images)

	return gui.Panels.Images.RerenderList()
}

// refreshImagesQuiet is the background poll. Images only change when someone
// pulls or deletes one, so it runs far less often than the instance list.
func (gui *Gui) refreshImagesQuiet() error {
	if err := gui.refreshImages(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

func (gui *Gui) handleImageDelete(g *gocui.Gui, v *gocui.View) error {
	image, err := gui.Panels.Images.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteImage, image.Label())

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := image.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshImages()
		})
	}, nil)
}
