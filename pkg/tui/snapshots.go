package tui

import (
	"slices"
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// snapshotsFollow is pkg/gui's SnapshotsLabel and SnapshotsInstances.
type snapshotsFollow struct {
	label     string
	instances []*commands.Instance
}

// snapshotsSpan is pkg/gui's.
type snapshotsSpan struct {
	instances bool
	projects  bool
}

// followSelection is pkg/gui's OnSelect into refreshSnapshotsFor.
func (m *Model) followSelection() {
	if m.currentSideView() != "instances" {
		return
	}

	instance := m.selectedInstance()
	if instance == nil {
		return
	}

	following := m.snapshotsFollow.instances
	if len(following) == 1 && following[0].Key() == instance.Key() && following[0].Remote == instance.Remote {
		return
	}

	m.snapshotsFollow = snapshotsFollow{label: m.onRemote(instance.Name, instance.Remote), instances: []*commands.Instance{instance}}
	if !m.snapshotsShowAll {
		m.setSnapshots()
	}
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

	m.snapshotsSpan = snapshotsSpan{instances: len(instances) > 1, projects: spansProjects(instances, instanceProject)}

	var snapshots []*commands.Snapshot
	for _, instance := range instances {
		snapshots = append(snapshots, instance.Latest().Snapshots()...)
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
	if snapshot.Instance != nil {
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
