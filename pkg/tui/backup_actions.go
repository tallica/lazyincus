package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

func onBackup(action func(*Model, *commands.ComposeBackup) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		backup := m.selectedBackup()
		if backup == nil {
			return nil
		}

		return action(m, backup)
	}
}

func (m *Model) selectedStackTarget() (composeTarget, bool) {
	stack := m.selectedStack
	if stack == nil || stack.Name == "" {
		return composeTarget{}, false
	}

	return stackTarget(stack), true
}

// backupRun is composeRun for a backup verb, which changes the volumes too.
func (m *Model) backupRun(target composeTarget, args ...string) tea.Cmd {
	return m.backupRunThen(target, m.fetchBackups, args...)
}

func (m *Model) backupRunThen(target composeTarget, backups func() fetch, args ...string) tea.Cmd {
	return m.composeRunThen(target, append([]string{"backup"}, args...), func() tea.Cmd {
		return refresh(backups(), m.fetchVolumes())
	})
}

// backupCreate moves the cursor to the new backup: incus-compose picks the timestamp, so it is the one the list lacked.
func (m *Model) backupCreate(target composeTarget, args ...string) tea.Cmd {
	before := map[string]bool{}
	for _, backup := range m.backups {
		before[backup.Timestamp] = true
	}

	return m.backupRunThen(target, func() fetch {
		listing := m.fetchBackups()

		return func() (func(), error) {
			apply, err := listing()
			if err != nil {
				return nil, err
			}

			return func() {
				apply()
				m.selectNewBackup(before)
			}, nil
		}
	}, append([]string{"create"}, args...)...)
}

func (m *Model) selectNewBackup(before map[string]bool) {
	p := m.panelNamed("backups")

	for row, item := range p.shown {
		if !before[m.backups[item].Timestamp] {
			p.selectRow(row, m.listRows(p))

			return
		}
	}
}

// backupNew: without --live incus-compose stops the services, so the menu saying so is the confirmation.
func (m *Model) backupNew() tea.Cmd {
	target, ok := m.selectedStackTarget()
	if !ok {
		return nil
	}

	label := m.onRemote(target.project, target.remote)

	m.openOptionalTextPrompt(fmt.Sprintf(m.tr.BackupNamePrompt, label), m.tr.BackupNameHint, func(m *Model, name string) tea.Cmd {
		var args []string
		if name != "" {
			args = append(args, "--name", name)
		}

		m.openMenu(menuOptions{
			title: fmt.Sprintf(m.tr.BackupModeMenuTitle, label),
			items: []*menuItem{
				{label: m.tr.BackupStopped, onPress: func(m *Model) tea.Cmd { return m.backupCreate(target, args...) }},
				{label: m.tr.BackupLive, onPress: func(m *Model) tea.Cmd { return m.backupCreate(target, append(args, "--live")...) }},
			},
		})

		return nil
	})

	return nil
}

func (m *Model) backupDelete(backup *commands.ComposeBackup) tea.Cmd {
	target, ok := m.selectedStackTarget()
	if !ok {
		return nil
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.DeleteBackup, presentation.BackupLabel(backup)), func() tea.Cmd {
		return m.backupRun(target, "delete", backup.Timestamp)
	})

	return nil
}

func (m *Model) backupPrune() tea.Cmd {
	target, ok := m.selectedStackTarget()
	if !ok {
		return nil
	}

	label := m.onRemote(target.project, target.remote)

	m.openTextPrompt(fmt.Sprintf(m.tr.PruneBackupsPrompt, label), m.tr.PruneBackupsHint, "", func(m *Model, input string) tea.Cmd {
		keep, err := strconv.Atoi(input)
		if err != nil || keep < 0 {
			m.openError(fmt.Sprintf(m.tr.PruneNotANumber, input))

			return nil
		}

		count := len(m.backups)
		if count <= keep {
			m.openError(fmt.Sprintf(m.tr.NoBackupsToPrune, label, count, keep))

			return nil
		}

		m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.ConfirmPruneBackups, count-keep, label, keep), func() tea.Cmd {
			return m.backupRun(target, "delete", "--keep-last", strconv.Itoa(keep))
		})

		return nil
	})

	return nil
}

// backupRestore has no confirmation of its own: incus-compose asks on the terminal, and refuses while running.
func (m *Model) backupRestore(backup *commands.ComposeBackup) tea.Cmd {
	target, ok := m.selectedStackTarget()
	if !ok {
		return nil
	}

	items := []*menuItem{{
		label:   fmt.Sprintf(m.tr.RestoreWholeStack, m.onRemote(target.project, target.remote)),
		onPress: func(m *Model) tea.Cmd { return m.backupRun(target, "restore", backup.Timestamp) },
	}}

	for _, service := range m.backupServices() {
		items = append(items, &menuItem{
			label: fmt.Sprintf(m.tr.RestoreService, service.Name),
			onPress: func(m *Model) tea.Cmd {
				serviceTarget := target
				serviceTarget.service = service.Name

				return m.backupRun(serviceTarget, "restore", backup.Timestamp)
			},
		})
	}

	m.openMenu(menuOptions{title: fmt.Sprintf(m.tr.RestoreBackupMenuTitle, presentation.BackupLabel(backup)), items: items})

	return nil
}

// backupServices are the services a restore can be narrowed to, the selected one first.
func (m *Model) backupServices() []*commands.ComposeService {
	services := slices.DeleteFunc(m.composeServices(), func(service *commands.ComposeService) bool {
		return len(service.NamedVolumes()) == 0
	})

	row := m.selectedServiceRow()
	if row == nil {
		return services
	}

	rank := func(service *commands.ComposeService) int {
		if service.Name == row.Service.Name {
			return 0
		}

		return 1
	}
	slices.SortStableFunc(services, func(a, b *commands.ComposeService) int { return cmp.Compare(rank(a), rank(b)) })

	return services
}

func (m *Model) backupVerify(backup *commands.ComposeBackup) tea.Cmd {
	stack := m.selectedStack
	if stack == nil || stack.Name == "" {
		return nil
	}

	command, err := m.commandFor(stack.Remote)
	if err != nil {
		m.openError(err.Error())

		return nil
	}

	key := m.backupKey(backup)

	var verification *commands.BackupVerification

	return m.withWaitingStatusThen(m.tr.VerifyingStatus, func() error {
		var err error
		verification, err = command.VerifyComposeBackup(stack.Dir, backup.Timestamp)

		return err
	}, m.reportOr(func() tea.Cmd {
		if m.backupVerifications == nil {
			m.backupVerifications = map[string]*commands.BackupVerification{}
		}

		m.backupVerifications[key] = verification
		m.setBackups(m.backups)

		return nil
	}))
}

// goToBackups moves the session to a stack elsewhere first: the tab is only there for a stack on the session's remote.
func (m *Model) goToBackups() tea.Cmd {
	focus := func() tea.Cmd {
		if !m.backupsAway() {
			m.focusView("backups")
		}

		return nil
	}

	stack := m.selectedStackRow()
	if stack == nil {
		return focus()
	}

	return m.stackSwitchRemoteThen(stack, focus)
}
