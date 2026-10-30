package tui

import (
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) fetchVolumes() fetch {
	ticket := m.refreshes.volumes.issue()

	return func() (func(), error) {
		volumes, err := m.incus.GetVolumes()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.volumes.admit(ticket) {
				m.setVolumes(volumes)
			}
		}, nil
	}
}

func (m *Model) setVolumes(volumes []*commands.Volume) {
	m.refill(m.panelNamed("volumes"), func(item int) string { return m.volumes[item].Key() }, func() [][]string {
		sort.SliceStable(volumes, func(a, b int) bool { return sortVolumes(volumes[a], volumes[b]) })
		m.volumes = volumes
		m.volumesSpan = spansProjects(volumes, volumeProject)

		cells := make([][]string, len(volumes))
		for i, volume := range volumes {
			cells[i] = presentation.GetVolumeDisplayStrings(volume, m.volumesSpan)
		}

		return cells
	})

	// A followed volume's snapshots come with the volumes.
	if m.snapshotsVolume() != nil {
		m.setSnapshots()
	}
}

// sortVolumes is pkg/gui's: by pool, custom volumes ahead of Incus's own.
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

func volumeProject(volume *commands.Volume) string { return volume.Volume.Project }

func (m *Model) volumeFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{Index: projectColumns(m.volumesSpan), MinWidth: presentation.MinVolumeNameWidth}}
}

func (m *Model) selectedVolume() *commands.Volume {
	item := m.panelNamed("volumes").item()
	if item < 0 || item >= len(m.volumes) {
		return nil
	}

	return m.volumes[item]
}

func (m *Model) volumeMainTabs() []mainTab {
	return []mainTab{{key: "config", title: m.tr.ConfigTitle, task: m.volumeConfigTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

func (m *Model) volumeConfigTask() mainTaskFunc {
	volume, tr := m.selectedVolume(), m.tr
	loc := m.sessionLocation(volume.Volume.Project)

	return renderString(func() string { return presentation.VolumeConfig(volume, loc, tr) })
}

// volumeKey is pkg/gui's, leaving out the usage, which changes with every poll.
func (m *Model) volumeKey() string {
	volume := m.selectedVolume()
	if volume == nil {
		return ""
	}

	return volume.Key() + "-" + utils.Fingerprint(volume.Volume)
}
