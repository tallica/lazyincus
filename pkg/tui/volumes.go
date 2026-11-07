package tui

import (
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) newVolumes(p *panel) list[*commands.Volume] {
	return list[*commands.Volume]{
		panel: p, seq: &m.refreshes.volumes,
		get: func(incus *commands.IncusCommand, _ *commands.Listing) ([]*commands.Volume, error) {
			return incus.GetVolumes()
		},
		less:    sortVolumes,
		key:     (*commands.Volume).Key,
		project: func(volume *commands.Volume) string { return volume.Volume.Project },
		cells:   presentation.GetVolumeDisplayStrings,
		// Leaving out the usage, which changes with every poll.
		version: func(volume *commands.Volume) string { return utils.Fingerprint(volume.Volume) },
		// A followed volume's snapshots come with the volumes.
		filled: func() {
			if m.snapshotsVolume() != nil {
				m.setSnapshots()
			}
		},
	}
}

// sortVolumes: by pool, custom volumes ahead of Incus's own.
func sortVolumes(a, b *commands.Volume) bool {
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

func (m *Model) volumeFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{Index: projectColumns(m.volumes.span), MinWidth: presentation.MinVolumeNameWidth}}
}

func (m *Model) volumeMainTabs() []mainTab {
	return []mainTab{{key: "config", title: m.tr.ConfigTitle, task: m.volumeConfigTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

func (m *Model) volumeConfigTask() mainTaskFunc {
	volume, tr := m.volumes.selected(), m.tr
	loc := m.sessionLocation(volume.Volume.Project)

	return renderString(func() string { return presentation.VolumeConfig(volume, loc, tr) })
}
