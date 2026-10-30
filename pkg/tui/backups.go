package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) backupsTitle() string {
	stack := m.selectedStack
	if stack == nil || stack.Name == "" {
		return m.tr.BackupsTitle
	}

	return fmt.Sprintf(m.tr.BackupsTitleProject, m.onRemote(stack.Name, stack.Remote))
}

// backupKey identifies a backup across stacks: its timestamp is only unique within its own.
func (m *Model) backupKey(backup *commands.ComposeBackup) string {
	return stackIdentity(m.selectedStack) + "\x00" + backup.Timestamp
}

func (m *Model) backupVerification(backup *commands.ComposeBackup) *commands.BackupVerification {
	return m.backupVerifications[m.backupKey(backup)]
}

func (m *Model) selectedBackup() *commands.ComposeBackup {
	item := m.panelNamed("backups").item()
	if item < 0 || item >= len(m.backups) {
		return nil
	}

	return m.backups[item]
}

// backupItemKey has the verification in it so that a `v` redraws the tab.
func (m *Model) backupItemKey() string {
	backup := m.selectedBackup()
	if backup == nil {
		return ""
	}

	return "backups-" + m.backupKey(backup) + "-" + utils.Fingerprint(m.backupVerification(backup))
}

func (m *Model) backupMainTabs() []mainTab {
	return []mainTab{{key: "info", title: m.tr.InfoTitle, task: m.backupInfoTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

func (m *Model) backupInfoTask() mainTaskFunc {
	backup, stack, width, tr := m.selectedBackup(), m.selectedStack, m.main.width, m.tr
	verification := m.backupVerification(backup)

	var loc *presentation.Location
	if stack != nil {
		remote := m.stackRemote(stack)
		loc = &presentation.Location{Remote: remote, Project: stack.Name, Stack: m.stackLabel(remote, stack.Name)}
	}

	return renderString(func() string { return presentation.BackupInfo(loc, backup, verification, width, tr) })
}

func (m *Model) backupFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{Index: presentation.BackupNameColumn, MinWidth: presentation.MinBackupNameWidth}}
}

// fetchBackups puts why it can't list in the empty list, not a popup: a stack never backed up fails the same way on every poll.
func (m *Model) fetchBackups() fetch {
	if m.composeUnavailable() {
		return func() (func(), error) { return func() {}, nil }
	}

	ticket := m.refreshes.backups.issue()
	stack := m.selectedStack

	if stack == nil || stack.Name == "" {
		return func() (func(), error) { return m.showBackups(ticket, nil, m.tr.NoStackSelected), nil }
	}

	return func() (func(), error) {
		command, err := m.commandFor(stack.Remote)
		if err == nil && !m.onSessionRemote(stack.Remote) {
			err = m.remotes.remoteStatusErr(stack.Remote)
		}

		var backups []*commands.ComposeBackup
		if err == nil {
			backups, err = m.loadBackups(command, stack.Dir)
		}

		if err != nil {
			m.incus.Log.Warn().Err(err).Send()

			return m.showBackups(ticket, nil, fmt.Sprintf(m.tr.BackupsUnavailable, firstLine(err.Error()))), nil
		}

		return m.showBackups(ticket, backups, m.tr.NoBackups), nil
	}
}

func (m *Model) showBackups(ticket uint64, backups []*commands.ComposeBackup, empty string) func() {
	return func() {
		if !m.refreshes.backups.admit(ticket) {
			return
		}

		m.panelNamed("backups").empty = empty
		m.setBackups(backups)
	}
}

func (m *Model) setBackups(backups []*commands.ComposeBackup) {
	m.refill(m.panelNamed("backups"), func(item int) string { return m.backups[item].Timestamp }, func() [][]string {
		slices.SortStableFunc(backups, func(a, b *commands.ComposeBackup) int { return b.CreatedAt().Compare(a.CreatedAt()) })
		m.backups = backups

		cells := make([][]string, len(backups))
		for i, backup := range backups {
			cells[i] = presentation.GetBackupDisplayStrings(backup, m.backupVerification(backup), m.tr)
		}

		return cells
	})
}

func (m *Model) followStackBackups() {
	m.refreshes.backups.invalidate()

	p := m.panelNamed("backups")
	p.empty, p.selected = m.tr.NoBackups, 0
	m.refill(p, func(int) string { return "" }, func() [][]string {
		m.backups = nil

		return nil
	})
	m.queue(refresh(m.fetchBackups()))
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return line
}
