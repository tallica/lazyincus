package tui

import (
	"slices"
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// snapshotsFollow is pkg/gui's SnapshotsLabel, SnapshotsInstances and SnapshotsVolume.
type snapshotsFollow struct {
	label     string
	instances []*commands.Instance
	// volume is the custom volume's key, held by key: each refresh builds new values.
	volume string
}

// snapshotsSpan is pkg/gui's.
type snapshotsSpan struct {
	instances bool
	projects  bool
}

// followSelection is pkg/gui's OnSelect into refreshSnapshotsFor.
func (m *Model) followSelection() {
	switch m.currentSideView() {
	case "stacks":
		m.followStack(m.selectedStackRow())
		// A stack hands Snapshots nothing, so it empties rather than keep the last list's.
		if f := m.snapshotsFollow; f.label != "" || len(f.instances) > 0 || f.volume != "" {
			m.followSnapshotsOf(snapshotsFollow{})
		}
	case "services":
		m.followServices()
	case "instances":
		m.followInstance()
	case "volumes":
		// The other volumes' snapshots are their instance's.
		if volume := m.selectedVolume(); volume != nil && volume.IsCustom() && volume.Key() != m.snapshotsFollow.volume {
			m.followSnapshotsOf(snapshotsFollow{label: volume.Name, volume: volume.Key()})
		}
	}
}

func (m *Model) followInstance() {
	instance := m.selectedInstance()
	if instance == nil {
		return
	}

	following := m.snapshotsFollow.instances
	if len(following) == 1 && following[0].Key() == instance.Key() && following[0].Remote == instance.Remote {
		return
	}

	m.followSnapshotsOf(snapshotsFollow{label: m.onRemote(instance.Name, instance.Remote), instances: []*commands.Instance{instance}})
}

// followSnapshotsOf is pkg/gui's refreshSnapshotsFor and refreshSnapshotsForVolume.
func (m *Model) followSnapshotsOf(follow snapshotsFollow) {
	m.snapshotsFollow = follow
	if !m.snapshotsShowAll {
		m.setSnapshots()
	}
}

// snapshotsVolume is pkg/gui's: the followed volume as the newest refresh has it.
func (m *Model) snapshotsVolume() *commands.Volume {
	if m.snapshotsShowAll || m.snapshotsFollow.volume == "" {
		return nil
	}

	for _, volume := range m.volumes {
		if volume.Key() == m.snapshotsFollow.volume {
			return volume
		}
	}

	return nil
}

// setSnapshots is pkg/gui's renderSnapshots, the selection kept on its snapshot.
func (m *Model) setSnapshots() {
	key := ""
	if selected := m.selectedSnapshot(); selected != nil {
		key = selected.Key()
	}

	instances := m.snapshotsFollow.instances
	if m.snapshotsShowAll {
		instances = m.instances
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
	m.snapshots = snapshots

	p := m.panelNamed("snapshots")
	p.awaiting = ""
	p.cells = make([][]string, len(snapshots))
	for i, snapshot := range snapshots {
		p.cells[i] = presentation.GetSnapshotDisplayStrings(snapshot, m.snapshotOwner(snapshot))
	}
	m.filterList(p)
	if row := slices.IndexFunc(p.shown, func(item int) bool { return snapshots[item].Key() == key }); row >= 0 {
		p.selected = row
	}
	p.selectRow(p.selected, m.listRows(p))
}

// sortSnapshots is pkg/gui's: by instance, newest first within one.
func sortSnapshots(a, b *commands.Snapshot) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}

	if a.Owner != b.Owner {
		return a.Owner < b.Owner
	}

	return a.CreatedAt().After(b.CreatedAt())
}

func (m *Model) selectedSnapshot() *commands.Snapshot {
	item := m.panelNamed("snapshots").item()
	if item < 0 || item >= len(m.snapshots) {
		return nil
	}

	return m.snapshots[item]
}

// snapshotOwner is pkg/gui's.
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

// snapshotFlex is pkg/gui's: the instance gives way before the name.
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
	snapshot := m.selectedSnapshot()
	loc := presentation.Location{}
	switch {
	case snapshot.Volume != nil:
		loc = m.sessionLocation(snapshot.Project)
	case snapshot.Instance != nil:
		loc = m.instanceLocation(snapshot.Instance)
	}

	return renderString(func() string { return presentation.SnapshotConfig(snapshot, loc) })
}

func (m *Model) snapshotKey() string {
	snapshot := m.selectedSnapshot()
	if snapshot == nil {
		return ""
	}

	return snapshot.Key() + "-" + utils.Fingerprint(snapshot.Details())
}

// snapshotsLabel is pkg/gui's.
func (m *Model) snapshotsLabel() string {
	if m.snapshotsShowAll {
		return m.tr.AllSnapshotsLabel
	}

	return m.snapshotsFollow.label
}

// snapshotsTitle is pkg/gui's snapshotsTabTitle.
func (m *Model) snapshotsTitle() string {
	if label := m.snapshotsLabel(); label != "" {
		return m.tr.SnapshotsTitle + " (" + label + ")"
	}

	return m.tr.SnapshotsTitle
}

// toggleAllSnapshots is pkg/gui's handleToggleAllSnapshots.
func (m *Model) toggleAllSnapshots() {
	m.snapshotsShowAll = !m.snapshotsShowAll
	m.setSnapshots()
}

// snapshotsRemote is pkg/gui's actionRemote for the snapshots panel.
func (m *Model) snapshotsRemote() string {
	if instances := m.snapshotsFollow.instances; !m.snapshotsShowAll && len(instances) > 0 && instances[0].Remote != "" {
		return instances[0].Remote
	}

	return m.incus.RemoteName()
}
