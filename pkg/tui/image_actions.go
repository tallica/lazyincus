package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
)

func onImage(action func(*Model, *commands.Image) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		image := m.selectedImage()
		if image == nil {
			return nil
		}

		return action(m, image)
	}
}

// editInIncus: `incus ... edit` on the terminal, then the lists it changed.
func (m *Model) editInIncus(project string, fetches func() []fetch, args ...string) tea.Cmd {
	cmd := m.os.NewCmd("incus", append(projectCLIArgs(project), args...)...)

	return m.runSubprocess(cmd, "", func() tea.Cmd { return refreshQuiet(nil, fetches()...) })
}

func (m *Model) imageEdit(image *commands.Image) tea.Cmd {
	return m.editInIncus(image.Image.Project, func() []fetch { return []fetch{m.fetchImages()} }, "image", "edit", image.Fingerprint)
}

func (m *Model) imageDelete(image *commands.Image) tea.Cmd {
	message := fmt.Sprintf(m.tr.DeleteImage, m.qualified(image.Label(), image.Image.Project, m.imagesSpan))
	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, image.Delete, m.reportOr(func() tea.Cmd {
			return refresh(m.fetchImages())
		}))
	})

	return nil
}

func (m *Model) pruneImages() tea.Cmd {
	unused := lo.Filter(m.images, func(image *commands.Image, _ int) bool { return image.IsUnused() })
	cached := lo.Filter(unused, func(image *commands.Image, _ int) bool { return image.Image.Cached })

	item := func(format string, images []*commands.Image) *menuItem {
		return &menuItem{
			label:   fmt.Sprintf(format, len(images), imagesSize(images)),
			onPress: func(m *Model) tea.Cmd { return m.confirmPruneImages(images) },
		}
	}

	m.openMenu(menuOptions{title: m.tr.PruneImagesTitle, items: []*menuItem{
		item(m.tr.PruneCachedImages, cached),
		item(m.tr.PruneUnusedImages, unused),
	}})

	return nil
}

func imagesSize(images []*commands.Image) string {
	return units.GetByteSizeStringIEC(lo.SumBy(images, func(image *commands.Image) int64 { return image.Image.Size }), 2)
}

// confirmPruneImages names every image: a count doesn't say whether the one you meant to keep is among them.
func (m *Model) confirmPruneImages(images []*commands.Image) tea.Cmd {
	if len(images) == 0 {
		m.openError(m.tr.NothingToPrune)
		return nil
	}

	names := lo.Map(images, func(image *commands.Image, _ int) string {
		return m.qualified(image.Label()+" "+image.ShortFingerprint(), image.Image.Project, m.imagesSpan)
	})
	message := fmt.Sprintf(m.tr.ConfirmPruneImages, len(images), imagesSize(images), strings.Join(names, "\n"))

	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, func() error {
			var failed []error
			for _, image := range images {
				if err := image.Delete(); err != nil {
					failed = append(failed, errors.New(image.Label()+": "+err.Error()))
				}
			}

			return errors.Join(failed...)
		}, func(err error) tea.Cmd {
			if err != nil {
				m.openError(err.Error())
			}

			return refresh(m.fetchImages())
		})
	})

	return nil
}

func (m *Model) imageUsers(image *commands.Image) tea.Cmd {
	return m.showUsers(image.Label(), image.IsUsedBy)
}

func (m *Model) imageCopy(image *commands.Image) tea.Cmd {
	m.copyMenu(
		copyValue{m.tr.CopyFingerprint, image.Fingerprint},
		copyValue{m.tr.CopyAlias, image.Alias()},
	)

	return nil
}
