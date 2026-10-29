package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// onSnapshot is pkg/gui's onSelected for the snapshots panel.
func onSnapshot(action func(*Model, *commands.Snapshot) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		snapshot := m.selectedSnapshot()
		if snapshot == nil {
			return nil
		}

		return action(m, snapshot)
	}
}

// selectSnapshot is pkg/gui's Select: the cursor onto the snapshot with key, if shown.
func (m *Model) selectSnapshot(key string) bool {
	p := m.panelNamed("snapshots")
	for row, item := range p.shown {
		if m.snapshots[item].Key() == key {
			p.selectRow(row, m.listRows(p))
			return true
		}
	}

	return false
}

// snapshotOwnerName is pkg/gui's.
func (m *Model) snapshotOwnerName(snapshot *commands.Snapshot) string {
	spans := m.showProject || m.snapshotsSpan.projects
	if snapshot.Volume != nil {
		spans = m.volumesSpan
	}

	owner := m.qualified(snapshot.Owner, snapshot.Project, spans)
	if snapshot.Instance != nil {
		owner = m.onRemote(owner, snapshot.Instance.Remote)
	}

	return owner
}

// reportOr is a waiting status's then: f's error in a popup, else next.
func (m *Model) reportOr(next func() tea.Cmd) func(error) tea.Cmd {
	return func(err error) tea.Cmd {
		if err != nil {
			m.openError(err.Error())
			return nil
		}

		return next()
	}
}

// snapshotRestore is pkg/gui's; Incus restarts a running instance to
// restore it, so the row reads restoring until it's listed again.
func (m *Model) snapshotRestore(snapshot *commands.Snapshot) tea.Cmd {
	message := fmt.Sprintf(m.tr.RestoreSnapshot, m.snapshotOwnerName(snapshot), snapshot.Name)
	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		if snapshot.Volume != nil {
			return m.withWaitingStatusThen(m.tr.RestoringStatus, snapshot.Restore, m.reportOr(func() tea.Cmd {
				return refresh(m.snapshotOwnerFetches(snapshot)...)
			}))
		}

		return m.inTransition(m.tr.RestoringStatus, snapshot.Instance, "Restoring", snapshot.Restore)
	})

	return nil
}

func (m *Model) snapshotDelete(snapshot *commands.Snapshot) tea.Cmd {
	message := fmt.Sprintf(m.tr.DeleteSnapshot, snapshot.Name, m.snapshotOwnerName(snapshot))
	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, snapshot.Delete, m.reportOr(func() tea.Cmd {
			return refresh(m.snapshotOwnerFetches(snapshot)...)
		}))
	})

	return nil
}

// snapshotRename is pkg/gui's, the cursor following the snapshot to where its new name sorts.
func (m *Model) snapshotRename(snapshot *commands.Snapshot) tea.Cmd {
	title := fmt.Sprintf(m.tr.RenameSnapshotPrompt, snapshot.Name, m.snapshotOwnerName(snapshot))
	m.openTextPrompt(title, m.tr.RenameHint, snapshot.Name, func(m *Model, name string) tea.Cmd {
		if name == snapshot.Name {
			return nil
		}

		renamed := (&commands.Snapshot{Project: snapshot.Project, Owner: snapshot.Owner, Name: name, Volume: snapshot.Volume}).Key()

		return m.withWaitingStatusThen(m.tr.RenamingStatus, func() error { return snapshot.Rename(name) }, m.reportOr(func() tea.Cmd {
			return refresh(append(m.snapshotOwnerFetches(snapshot), func() (func(), error) {
				return func() { m.selectSnapshot(renamed) }, nil
			})...)
		}))
	})

	return nil
}

// snapshotNewInstance is pkg/gui's: `incus copy`, which drops the source's
// volatile keys by rules internal to Incus, then the new instance focused.
func (m *Model) snapshotNewInstance(snapshot *commands.Snapshot) tea.Cmd {
	if snapshot.Instance == nil {
		m.openError(m.tr.CannotCopyVolumeSnapshot)
		return nil
	}

	// The copy keeps the source's user keys, its service label among them.
	if snapshot.Instance.ComposeService() != "" {
		m.openError(fmt.Sprintf(m.tr.CannotCopyComposeSnapshot, snapshot.Owner))
		return nil
	}

	title := fmt.Sprintf(m.tr.NewInstancePrompt, m.snapshotOwnerName(snapshot), snapshot.Name)
	m.openTextPrompt(title, m.tr.NewInstanceHint, snapshot.Owner+"-"+snapshot.Name, func(m *Model, name string) tea.Cmd {
		cmd := m.instanceCmd(snapshot.Instance, "copy", snapshot.Owner+"/"+snapshot.Name, name)
		run := m.runIncus
		created := (&commands.Instance{Name: name, Project: snapshot.Instance.Project}).Key()

		return m.withWaitingStatusThen(m.tr.CopyingStatus, func() error {
			_, err := run(cmd)
			return err
		}, m.reportOr(func() tea.Cmd {
			return refresh(append(m.instanceFetches(), func() (func(), error) {
				return func() { m.focusInstance(created) }, nil
			})...)
		}))
	})

	return nil
}

// focusInstance is pkg/gui's Select then switchFocus, nothing while it's hidden.
func (m *Model) focusInstance(key string) {
	if m.selectInstance(key) {
		m.focusView("instances")
	}
}

// snapshotOwnerFetches is pkg/gui's refreshSnapshotOwner: what carries the snapshot.
func (m *Model) snapshotOwnerFetches(snapshot *commands.Snapshot) []fetch {
	if snapshot.Volume != nil {
		return []fetch{m.fetchVolumes()}
	}

	return m.instanceFetches()
}

func (m *Model) snapshotCopy(snapshot *commands.Snapshot) tea.Cmd {
	m.copyMenu(
		copyValue{m.tr.CopyName, snapshot.Name},
		copyValue{m.tr.CopySnapshotRef, snapshot.Owner + "/" + snapshot.Name},
	)

	return nil
}
