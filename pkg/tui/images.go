package tui

import (
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) newImages(p *panel) list[*commands.Image] {
	return list[*commands.Image]{
		panel: p, seq: &m.refreshes.images,
		get:     (*commands.IncusCommand).GetImages,
		less:    sortImages,
		key:     (*commands.Image).Key,
		project: func(image *commands.Image) string { return image.Image.Project },
		cells: func(image *commands.Image, span bool) []string {
			newer := m.upstreams[image.Fingerprint].State == commands.UpstreamNewer

			return presentation.GetImageDisplayStrings(image, span, newer)
		},
		version: func(image *commands.Image) string {
			upstream := presentation.ImageUpstream(m.upstreams[image.Fingerprint], m.tr)

			return utils.Fingerprint(image.Image, image.UsedBy, upstream)
		},
	}
}

// sortImages: by the label shown, then fingerprint, then project.
func sortImages(a, b *commands.Image) bool {
	left, right := strings.ToLower(a.Label()), strings.ToLower(b.Label())
	if left != right {
		return left < right
	}

	if a.Fingerprint != b.Fingerprint {
		return a.Fingerprint < b.Fingerprint
	}

	return a.Image.Project < b.Image.Project
}

func (m *Model) imageFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{Index: projectColumns(m.images.span), MinWidth: presentation.MinImageLabelWidth}}
}

// projectColumns is how many columns lead with the project.
func projectColumns(spans bool) int {
	if spans {
		return 1
	}

	return 0
}

func (m *Model) imageMainTabs() []mainTab {
	return []mainTab{{key: "config", title: m.tr.ConfigTitle, task: m.imageConfigTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

// imageConfigTask reads the registry check on the loop, for the task that renders off it.
func (m *Model) imageConfigTask() mainTaskFunc {
	image, tr := m.images.selected(), m.tr
	loc := m.sessionLocation(image.Image.Project)
	upstream := presentation.ImageUpstream(m.upstreams[image.Fingerprint], tr)

	return renderString(func() string { return presentation.ImageConfig(image, loc, upstream, tr) })
}

// sessionLocation: a resource only ever shows the session's remote.
func (m *Model) sessionLocation(project string) presentation.Location {
	return presentation.Location{Remote: m.incus.RemoteName(), Project: project}
}
