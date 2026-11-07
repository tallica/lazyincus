package tui

import (
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// snapshotsFollow: SnapshotsInstances and SnapshotsVolume.
type snapshotsFollow struct {
	label     string
	instances []*commands.Instance
	// volume is the custom volume's key, held by key: each refresh builds new values.
	volume string
}

type snapshotsSpan struct {
	instances bool
	projects  bool
}

func (m *Model) followSelection() {
	switch m.currentSideView() {
	case stacksView:
		m.followStack(m.selectedStackRow())
		// A stack hands Snapshots nothing, so it empties rather than keep the last list's.
		if f := m.snapshotsFollow; f.label != "" || len(f.instances) > 0 || f.volume != "" {
			m.followSnapshotsOf(snapshotsFollow{})
		}
	case servicesView:
		m.followServices()
	case instancesView:
		m.followInstance()
	case volumesView:
		// The other volumes' snapshots are their instance's.
		if volume := m.volumes.selected(); volume != nil && volume.IsCustom() && volume.Key() != m.snapshotsFollow.volume {
			m.followSnapshotsOf(snapshotsFollow{label: volume.Name, volume: volume.Key()})
		}
	}
}

func (m *Model) followInstance() {
	instance := m.instances.selected()
	if instance == nil {
		return
	}

	following := m.snapshotsFollow.instances
	if len(following) == 1 && following[0].Key() == instance.Key() && following[0].Remote == instance.Remote {
		return
	}

	m.followSnapshotsOf(snapshotsFollow{label: m.onRemote(instance.Name, instance.Remote), instances: []*commands.Instance{instance}})
}

func (m *Model) followSnapshotsOf(follow snapshotsFollow) {
	m.snapshotsFollow = follow
	if !m.snapshotsShowAll {
		m.setSnapshots()
	}
}

// snapshotsVolume: the followed volume as the newest refresh has it.
func (m *Model) snapshotsVolume() *commands.Volume {
	if m.snapshotsShowAll || m.snapshotsFollow.volume == "" {
		return nil
	}

	for _, volume := range m.volumes.items {
		if volume.Key() == m.snapshotsFollow.volume {
			return volume
		}
	}

	return nil
}

// newSnapshots: no fetch, as the snapshots come with what they follow.
func (m *Model) newSnapshots(p *panel) list[*commands.Snapshot] {
	return list[*commands.Snapshot]{
		panel:   p,
		key:     (*commands.Snapshot).Key,
		project: func(snapshot *commands.Snapshot) string { return snapshot.Project },
		version: func(snapshot *commands.Snapshot) string { return utils.Fingerprint(snapshot.Details()) },
	}
}

// setSnapshots spans instances and projects, which a list's span can't say.
func (m *Model) setSnapshots() {
	instances := m.snapshotsFollow.instances
	if m.snapshotsShowAll {
		instances = m.instances.items
	}

	var snapshots []*commands.Snapshot
	if volume := m.snapshotsVolume(); volume != nil {
		m.snapshotsSpan = snapshotsSpan{}
		snapshots = volume.Snapshots()
	} else {
		m.snapshotsSpan = snapshotsSpan{instances: len(instances) > 1, projects: spansProjects(instances, instanceProject)}
		for _, instance := range instances {
			snapshots = append(snapshots, instance.Latest().Snapshots()...)
		}
	}
	sort.SliceStable(snapshots, func(a, b int) bool { return sortSnapshots(snapshots[a], snapshots[b]) })

	m.refill(m.snapshots.panel, m.snapshots.keyOf, func() [][]string {
		m.snapshots.items = snapshots

		cells := make([][]string, len(snapshots))
		for i, snapshot := range snapshots {
			cells[i] = presentation.GetSnapshotDisplayStrings(snapshot, m.snapshotOwner(snapshot))
		}

		return cells
	})
}

// sortSnapshots: by instance, newest first within one.
func sortSnapshots(a, b *commands.Snapshot) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}

	if a.Owner != b.Owner {
		return a.Owner < b.Owner
	}

	return a.CreatedAt().After(b.CreatedAt())
}

func (m *Model) snapshotOwner(snapshot *commands.Snapshot) string {
	switch {
	case m.snapshotsSpan.projects:
		return snapshot.Project + "/" + snapshot.Owner
	case m.snapshotsSpan.instances:
		return snapshot.Owner
	default:
		return ""
	}
}

// snapshotFlex: the instance gives way before the name.
func (m *Model) snapshotFlex() []utils.FlexColumn {
	name := utils.FlexColumn{Index: 0, MinWidth: presentation.MinSnapshotNameWidth}
	if !m.snapshotsSpan.instances {
		return []utils.FlexColumn{name}
	}

	return []utils.FlexColumn{{Index: 1, MinWidth: presentation.MinSnapshotOwnerWidth}, name}
}

func (m *Model) snapshotMainTabs() []mainTab {
	return []mainTab{{key: "config", title: m.tr.ConfigTitle, task: m.snapshotConfigTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

func (m *Model) snapshotConfigTask() mainTaskFunc {
	snapshot := m.snapshots.selected()
	loc := presentation.Location{}
	switch {
	case snapshot.Volume != nil:
		loc = m.sessionLocation(snapshot.Project)
	case snapshot.Instance != nil:
		loc = m.instanceLocation(snapshot.Instance)
	}

	return renderString(func() string { return presentation.SnapshotConfig(snapshot, loc) })
}

func (m *Model) snapshotsLabel() string {
	if m.snapshotsShowAll {
		return m.tr.AllSnapshotsLabel
	}

	return m.snapshotsFollow.label
}

func (m *Model) snapshotsTitle() string {
	if label := m.snapshotsLabel(); label != "" {
		return m.tr.SnapshotsTitle + " (" + label + ")"
	}

	return m.tr.SnapshotsTitle
}

func (m *Model) toggleAllSnapshots() {
	m.snapshotsShowAll = !m.snapshotsShowAll
	m.setSnapshots()
}

func (m *Model) snapshotsRemote() string {
	if instances := m.snapshotsFollow.instances; !m.snapshotsShowAll && len(instances) > 0 && instances[0].Remote != "" {
		return instances[0].Remote
	}

	return m.incus.RemoteName()
}
