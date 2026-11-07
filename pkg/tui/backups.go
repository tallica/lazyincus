package tui

import (
	"fmt"
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
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

// newBackups: Backups' fetch is its own, as it runs incus-compose for the selected stack.
func (m *Model) newBackups(p *panel) list[*commands.ComposeBackup] {
	return list[*commands.ComposeBackup]{
		panel: p,
		less:  func(a, b *commands.ComposeBackup) bool { return a.CreatedAt().After(b.CreatedAt()) },
		key:   m.backupKey,
		cells: func(backup *commands.ComposeBackup, _ bool) []string {
			return presentation.GetBackupDisplayStrings(backup, m.backupVerification(backup), m.tr)
		},
		// The verification, so that a `v` redraws the tab.
		version: func(backup *commands.ComposeBackup) string { return utils.Fingerprint(m.backupVerification(backup)) },
	}
}

func (m *Model) backupMainTabs() []mainTab {
	return []mainTab{{key: "info", title: m.tr.InfoTitle, task: m.backupInfoTask, wrap: m.cfg.UserConfig.Gui.WrapMainPanel}}
}

func (m *Model) backupInfoTask() mainTaskFunc {
	backup, stack, width, tr := m.backups.selected(), m.selectedStack, m.main.width, m.tr
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
	stack, src := m.selectedStack, m.sources()

	if stack == nil || stack.Name == "" {
		empty := src.tr.NoStackSelected

		return func() (func(), error) { return func() { m.showBackups(ticket, nil, empty) }, nil }
	}

	return func() (func(), error) {
		command, err := src.commandFor(stack.Remote)
		if err == nil && !src.onSessionRemote(stack.Remote) {
			err = src.remotes.remoteStatusErr(stack.Remote)
		}

		var backups []*commands.ComposeBackup
		if err == nil {
			backups, err = src.loadBackups(command, stack.Dir)
		}

		empty := src.tr.NoBackups
		if err != nil {
			src.incus.Log.Warn().Err(err).Send()

			backups, empty = nil, fmt.Sprintf(src.tr.BackupsUnavailable, firstLine(err.Error()))
		}

		return func() { m.showBackups(ticket, backups, empty) }, nil
	}
}

func (m *Model) showBackups(ticket uint64, backups []*commands.ComposeBackup, empty string) {
	if !m.refreshes.backups.admit(ticket) {
		return
	}

	m.panelNamed(backupsView).empty = empty
	m.backups.set(m, backups)
}

func (m *Model) followStackBackups() {
	m.refreshes.backups.invalidate()

	p := m.panelNamed(backupsView)
	p.empty, p.selected = m.tr.NoBackups, 0
	m.refill(p, func(int) string { return "" }, func() [][]string {
		m.backups.items = nil

		return nil
	})
	m.queue(refresh(m.fetchBackups()))
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return line
}
