package tui

import (
	"sort"
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) fetchImages() fetch {
	ticket := m.refreshes.images.issue()

	return func() (func(), error) {
		images, err := m.incus.GetImages()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.images.admit(ticket) {
				m.setImages(images)
			}
		}, nil
	}
}

func (m *Model) setImages(images []*commands.Image) {
	m.refill(m.panelNamed("images"), func(item int) string { return m.images[item].Key() }, func() [][]string {
		sort.SliceStable(images, func(a, b int) bool { return sortImages(images[a], images[b]) })
		m.images = images
		m.imagesSpan = spansProjects(images, imageProject)

		return m.imageCells()
	})
}

// sortImages: by the label shown, the fingerprint breaking ties.
func sortImages(a, b *commands.Image) bool {
	left, right := strings.ToLower(a.Label()), strings.ToLower(b.Label())
	if left != right {
		return left < right
	}

	return a.Fingerprint < b.Fingerprint
}

func imageProject(image *commands.Image) string { return image.Image.Project }

func (m *Model) imageCells() [][]string {
	cells := make([][]string, len(m.images))
	for i, image := range m.images {
		newer := m.upstreams[image.Fingerprint].State == commands.UpstreamNewer
		cells[i] = presentation.GetImageDisplayStrings(image, m.imagesSpan, newer)
	}

	return cells
}

func (m *Model) imageFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{Index: projectColumns(m.imagesSpan), MinWidth: presentation.MinImageLabelWidth}}
}

// projectColumns is how many columns lead with the project.
func projectColumns(spans bool) int {
	if spans {
		return 1
	}

	return 0
}

func (m *Model) selectedImage() *commands.Image {
	item := m.panelNamed("images").item()
	if item < 0 || item >= len(m.images) {
		return nil
	}

	return m.images[item]
}

func (m *Model) imageMainTabs() []mainTab {
	return []mainTab{{key: "config", title: m.tr.ConfigTitle, task: m.imageConfigTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

// imageConfigTask reads the registry check on the loop, for the task that renders off it.
func (m *Model) imageConfigTask() mainTaskFunc {
	image, tr := m.selectedImage(), m.tr
	loc := m.sessionLocation(image.Image.Project)
	upstream := presentation.ImageUpstream(m.upstreams[image.Fingerprint], tr)

	return renderString(func() string { return presentation.ImageConfig(image, loc, upstream, tr) })
}

func (m *Model) imageKey() string {
	image := m.selectedImage()
	if image == nil {
		return ""
	}

	upstream := presentation.ImageUpstream(m.upstreams[image.Fingerprint], m.tr)

	return image.Key() + "-" + utils.Fingerprint(image.Image, image.UsedBy, upstream)
}

// sessionLocation: a resource only ever shows the session's remote.
func (m *Model) sessionLocation(project string) presentation.Location {
	return presentation.Location{Remote: m.incus.RemoteName(), Project: project}
}
