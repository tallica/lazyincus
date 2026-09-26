package gui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/types"

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
				return "images-" + image.Key()
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
		SameItem: func(a, b *commands.Image) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(item *commands.Image) []string {
			return presentation.GetImageDisplayStrings(item, gui.State.SpansProjects.Images)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index: projectColumns(gui.State.SpansProjects.Images), MinWidth: presentation.MinImageLabelWidth,
			}}
		},
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
	output += utils.WithPadding("Last used: ", padding) + image.Image.LastUsedAt.String() + "\n"
	output += utils.WithPadding("Cached: ", padding) + fmt.Sprint(image.Image.Cached) + "\n"
	output += utils.WithPadding("Used by: ", padding) + gui.imageUsersStr(image) + "\n"

	data, err := utils.MarshalIntoYaml(image.Image)
	if err != nil {
		return fmt.Sprintf("Error marshalling image details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) fetchImages() (func() error, error) {
	ticket := gui.refreshes.images.issue()

	images, err := gui.IncusCommand.GetImages()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.images.admit(ticket) {
			return nil
		}

		gui.State.SpansProjects.Images = spansMultipleProjects(
			lo.Map(images, func(image *commands.Image, _ int) string { return image.Image.Project }))

		gui.Panels.Images.SetItems(images)

		return gui.Panels.Images.RerenderList()
	}, nil
}

func (gui *Gui) refreshImages() error {
	return gui.refresh(nil, gui.fetchImages)
}

// refreshImagesQuiet is the background poll. Images only change when someone
// pulls or deletes one, so it runs far less often than the instance list.
func (gui *Gui) refreshImagesQuiet() error {
	if err := gui.refreshImages(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// imageUsersStr names the instances, grouped by project, which is how they
// read in the instances panel.
func (gui *Gui) imageUsersStr(image *commands.Image) string {
	if image.UsersUnknown {
		return gui.Tr.ImageUsersUnknown
	}

	if image.IsUnused() {
		return gui.Tr.UsedByNothing
	}

	byProject := lo.GroupBy(image.UsedBy, func(user string) string {
		project, _, _ := strings.Cut(user, "/")
		return project
	})

	projects := lo.Keys(byProject)
	slices.Sort(projects)

	groups := lo.Map(projects, func(project string, _ int) string {
		names := lo.Map(byProject[project], func(user string, _ int) string {
			_, name, _ := strings.Cut(user, "/")
			return name
		})

		return fmt.Sprintf("%s (%s)", strings.Join(names, ", "), project)
	})

	return strings.Join(groups, "; ")
}

// handlePruneImages offers the two sizes of prune: the images Incus cached
// on a launch, which it expires by itself anyway, or every image nothing
// was created from - where an image copied in on purpose, incus-compose's
// among them, ends up once nothing runs it.
func (gui *Gui) handlePruneImages(g *gocui.Gui, v *gocui.View) error {
	unused := lo.Filter(gui.Panels.Images.List.GetAllItems(), func(image *commands.Image, _ int) bool {
		return image.IsUnused()
	})

	cached := lo.Filter(unused, func(image *commands.Image, _ int) bool { return image.Image.Cached })

	item := func(format string, images []*commands.Image) *types.MenuItem {
		return &types.MenuItem{
			Label:   fmt.Sprintf(format, len(images), imagesSize(images)),
			OnPress: func() error { return gui.confirmPruneImages(images) },
		}
	}

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.PruneImagesTitle,
		Items: []*types.MenuItem{
			item(gui.Tr.PruneCachedImages, cached),
			item(gui.Tr.PruneUnusedImages, unused),
		},
	})
}

func imagesSize(images []*commands.Image) string {
	return units.GetByteSizeStringIEC(lo.SumBy(images, func(image *commands.Image) int64 { return image.Image.Size }), 2)
}

// confirmPruneImages names every image it would delete: a count alone
// doesn't say whether the one you meant to keep is among them.
func (gui *Gui) confirmPruneImages(images []*commands.Image) error {
	if len(images) == 0 {
		return gui.createErrorPanel(gui.Tr.NothingToPrune)
	}

	names := lo.Map(images, func(image *commands.Image, _ int) string {
		return image.Label() + " " + image.ShortFingerprint()
	})

	prompt := fmt.Sprintf(gui.Tr.ConfirmPruneImages, len(images), imagesSize(images), strings.Join(names, "\n"))

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			failed := []string{}

			for _, image := range images {
				if err := image.Delete(); err != nil {
					failed = append(failed, image.Label()+": "+err.Error())
				}
			}

			if err := gui.refreshImages(); err != nil {
				return err
			}

			if len(failed) > 0 {
				return gui.createErrorPanel(strings.Join(failed, "\n"))
			}

			return nil
		})
	}, nil)
}

func (gui *Gui) showImageUsers(image *commands.Image) error {
	return gui.showUsers(image.Label(), image.IsUsedBy)
}

func (gui *Gui) imageDelete(image *commands.Image) error {
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
