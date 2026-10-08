package gui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// getBackupsPanel lists the selected stack's `incus-compose backup` runs.
func (gui *Gui) getBackupsPanel() *panels.SideListPanel[*commands.ComposeBackup] {
	return &panels.SideListPanel[*commands.ComposeBackup]{
		ContextState: &panels.ContextState[*commands.ComposeBackup]{
			GetMainTabs: func() []panels.MainTab[*commands.ComposeBackup] {
				return []panels.MainTab[*commands.ComposeBackup]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderBackupInfo,
					},
				}
			},
			// The verification is in the key so that a `v` redraws the tab.
			GetItemContextCacheKey: func(backup *commands.ComposeBackup) string {
				return "backups-" + gui.backupKey(backup) + "-" + utils.Fingerprint(gui.backupVerification(backup))
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeBackup]{
			List: panels.NewFilteredList[*commands.ComposeBackup](),
			View: gui.Views.Backups,
		},
		NoItemsMessage: gui.Tr.NoStackSelected,
		Gui:            gui.intoInterface(),
		Hide:           gui.composeUnavailable,
		Sort: func(a, b *commands.ComposeBackup) bool {
			return a.CreatedAt().After(b.CreatedAt())
		},
		SameItem: func(a, b *commands.ComposeBackup) bool {
			return a.Timestamp == b.Timestamp
		},
		GetTableCells: func(backup *commands.ComposeBackup) []string {
			return presentation.GetBackupDisplayStrings(backup, gui.backupVerification(backup), gui.Tr)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{Index: presentation.BackupNameColumn, MinWidth: presentation.MinBackupNameWidth}}
		},
	}
}

// backupsPanelTitle names the stack, as the services panel's title does.
func (gui *Gui) backupsPanelTitle() string {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return gui.Tr.BackupsTitle
	}

	return fmt.Sprintf(gui.Tr.BackupsTitleProject, gui.onRemote(stack.Name, stack.Remote))
}

// backupKey identifies a backup across stacks: its timestamp is only
// unique within its own.
func (gui *Gui) backupKey(backup *commands.ComposeBackup) string {
	return stackIdentity(gui.selectedStack.Load()) + "\x00" + backup.Timestamp
}

// backupVerification is the last `v` on the backup, nil for none.
func (gui *Gui) backupVerification(backup *commands.ComposeBackup) *commands.BackupVerification {
	return gui.State.BackupVerifications[gui.backupKey(backup)]
}

func (gui *Gui) renderBackupInfo(backup *commands.ComposeBackup) tasks.TaskFunc {
	verification := gui.backupVerification(backup)
	stack := gui.selectedStack.Load()

	return gui.NewSimpleRenderStringTask(func() string { return gui.backupInfoStr(stack, backup, verification) })
}

func (gui *Gui) backupInfoStr(stack *commands.ComposeStack, backup *commands.ComposeBackup, verification *commands.BackupVerification) string {
	padding := identityPadding
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := ""
	if stack != nil {
		remote := stack.Remote
		if remote == "" {
			remote = gui.IncusCommand.RemoteName()
		}

		output += gui.locationStr(location{remote: remote, project: stack.Name, stack: gui.stackLabel(remote, stack.Name)})
	}

	output += line("Name", backup.Name)
	output += line("Taken at", backup.CreatedAt().Local().Format(presentation.DateTimeFormat))
	// What every incus-compose backup verb takes to name this one.
	output += line("Timestamp", backup.Timestamp)
	output += line("Pool", backup.Pool())

	if backup.Size > 0 {
		output += line("Size", units.GetByteSizeStringIEC(backup.Size, 2))
	}

	if verification == nil {
		output += line("Verified", gui.Tr.BackupNotVerified)
	}

	output += "\n" + gui.sectionHeading(gui.Tr.VolumesTitle) + "\n\n"
	output += gui.backupVolumesStr(backup, verification)

	return output
}

// backupVolumesStr pairs each volume with its backup and `verify`'s word on
// it; one the stack has gained since the backup has no backup side.
func (gui *Gui) backupVolumesStr(backup *commands.ComposeBackup, verification *commands.BackupVerification) string {
	statuses := map[string]string{}
	if verification != nil {
		for _, volume := range verification.Volumes {
			statuses[volume.Volume] = volume.Status
		}
	}

	rows := make([][]string, 0, len(backup.Volumes))
	listed := map[string]bool{}

	for _, volume := range backup.Volumes {
		listed[volume.Source.Name] = true
		rows = append(rows, []string{volume.Source.Name, "→ " + volume.Backup.Project + "/" + volume.Backup.Name, presentation.BackupVolumeStatus(statuses[volume.Source.Name])})
	}

	if verification != nil {
		for _, volume := range verification.Volumes {
			if !listed[volume.Volume] {
				rows = append(rows, []string{volume.Volume, "", presentation.BackupVolumeStatus(volume.Status)})
			}
		}
	}

	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}

	return table
}

// fetchBackups lists the selected stack's backups. Not an error when it
// can't: the reason goes in the empty list instead, so that a stack never
// backed up, or on a remote that's away, isn't a popup on every refresh.
func (gui *Gui) fetchBackups() (func() error, error) {
	if gui.composeUnavailable() {
		return func() error { return nil }, nil
	}

	ticket := gui.refreshes.backups.issue()

	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return gui.showBackups(ticket, nil, gui.Tr.NoStackSelected), nil
	}

	command, err := gui.commandFor(stack.Remote)
	if err == nil && !gui.onSessionRemote(stack.Remote) {
		err = gui.remotes.remoteStatusErr(stack.Remote)
	}

	var backups []*commands.ComposeBackup
	if err == nil {
		backups, err = gui.loadBackups(command, stack.Dir)
	}

	if err != nil {
		gui.Log.Warn().Err(err).Send()

		return gui.showBackups(ticket, nil, fmt.Sprintf(gui.Tr.BackupsUnavailable, firstLine(err.Error()))), nil
	}

	return gui.showBackups(ticket, backups, gui.Tr.NoBackups), nil
}

func (gui *Gui) showBackups(ticket uint64, backups []*commands.ComposeBackup, empty string) func() error {
	return func() error {
		if !gui.refreshes.backups.admit(ticket) {
			return nil
		}

		gui.Panels.Backups.NoItemsMessage = empty
		gui.Panels.Backups.SetItems(backups)

		return gui.Panels.Backups.RerenderList()
	}
}

func (gui *Gui) refreshBackupsQuiet() error {
	if err := gui.refresh(nil, gui.fetchBackups); err != nil {
		gui.Log.Warn().Err(err).Send()
	}

	return nil
}

// followStackBackups empties the panel for a different stack and lists
// its own. Main loop only, from followStack.
func (gui *Gui) followStackBackups() error {
	gui.refreshes.backups.invalidate()

	gui.Panels.Backups.NoItemsMessage = gui.Tr.NoBackups
	gui.Panels.Backups.SetItems(nil)
	gui.Panels.Backups.SetSelectedLineIdx(0)

	if err := gui.Panels.Backups.RerenderList(); err != nil {
		return err
	}

	gui.refreshInBackground(gui.fetchBackups)

	return nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// selectedStackTarget is the stack a backup verb acts on.
func (gui *Gui) selectedStackTarget() (composeTarget, bool) {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return composeTarget{}, false
	}

	return stackTarget(stack), true
}

// backupRun is composeRun for a backup verb, which also changes the
// backups and the volumes holding them.
func (gui *Gui) backupRun(target composeTarget, args ...string) error {
	return gui.backupRunThen(target, nil, args...)
}

// backupRunThen is backupRun, then run on the main loop once the backups
// have been listed again.
func (gui *Gui) backupRunThen(target composeTarget, then func() error, args ...string) error {
	if err := gui.composeRun(target, append([]string{"backup"}, args...)...); err != nil {
		return err
	}

	go func() {
		if err := gui.refresh(then, gui.fetchBackups, gui.fetchVolumes); err != nil {
			gui.g.Update(func(*gocui.Gui) error { return err })
		}
	}()

	return nil
}

// backupCreate is `backup create`, the cursor moved to the backup it took.
// incus-compose picks the timestamp, so that's the newest one the list
// didn't have before; without this the cursor stays on the row it was on,
// which the new one has just pushed down.
func (gui *Gui) backupCreate(target composeTarget, args ...string) error {
	before := lo.SliceToMap(gui.Panels.Backups.List.GetAllItems(), func(backup *commands.ComposeBackup) (string, bool) {
		return backup.Timestamp, true
	})

	return gui.backupRunThen(target, func() error { return gui.selectNewBackup(before) },
		append([]string{"create"}, args...)...)
}

// selectNewBackup puts the cursor on the newest backup not in before.
func (gui *Gui) selectNewBackup(before map[string]bool) error {
	created, ok := lo.Find(gui.Panels.Backups.List.GetItems(), func(backup *commands.ComposeBackup) bool {
		return !before[backup.Timestamp]
	})
	if !ok || !gui.Panels.Backups.Select(created) {
		return nil
	}

	if err := gui.Panels.Backups.RerenderList(); err != nil {
		return err
	}

	return gui.Panels.Backups.HandleSelect()
}

// handleBackupCreate is `n`. Without --live, incus-compose stops the
// services for the backup, so the menu saying so is the confirmation.
func (gui *Gui) handleBackupCreate(g *gocui.Gui, v *gocui.View) error {
	target, ok := gui.selectedStackTarget()
	if !ok {
		return nil
	}

	label := gui.onRemote(target.project, target.remote)

	return gui.openOptionalTextPrompt(fmt.Sprintf(gui.Tr.BackupNamePrompt, label), gui.Tr.BackupNameHint, func(name string) error {
		var args []string
		if name != "" {
			args = append(args, "--name", name)
		}

		return gui.Menu(CreateMenuOptions{
			Title: fmt.Sprintf(gui.Tr.BackupModeMenuTitle, label),
			Items: []*types.MenuItem{
				{
					Label:   gui.Tr.BackupStopped,
					OnPress: func() error { return gui.backupCreate(target, args...) },
				},
				{
					Label:   gui.Tr.BackupLive,
					OnPress: func() error { return gui.backupCreate(target, append(args, "--live")...) },
				},
			},
		})
	})
}

func (gui *Gui) backupDelete(backup *commands.ComposeBackup) error {
	target, ok := gui.selectedStackTarget()
	if !ok {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteBackup, gui.backupLabel(backup))

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.backupRun(target, "delete", backup.Timestamp)
	}, nil)
}

// handleBackupPrune is `D`: `delete --keep-last`, with how many it deletes
// counted off the list before asking.
func (gui *Gui) handleBackupPrune(g *gocui.Gui, v *gocui.View) error {
	target, ok := gui.selectedStackTarget()
	if !ok {
		return nil
	}

	label := gui.onRemote(target.project, target.remote)

	return gui.openTextPrompt(fmt.Sprintf(gui.Tr.PruneBackupsPrompt, label), gui.Tr.PruneBackupsHint, "", func(input string) error {
		keep, err := strconv.Atoi(input)
		if err != nil || keep < 0 {
			return gui.createErrorPanel(fmt.Sprintf(gui.Tr.PruneNotANumber, input))
		}

		count := len(gui.Panels.Backups.List.GetAllItems())
		if count <= keep {
			return gui.createErrorPanel(fmt.Sprintf(gui.Tr.NoBackupsToPrune, label, count, keep))
		}

		prompt := fmt.Sprintf(gui.Tr.ConfirmPruneBackups, count-keep, label, keep)

		return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
			return gui.backupRun(target, "delete", "--keep-last", strconv.Itoa(keep))
		}, nil)
	})
}

// backupRestore is `r`. No --yes and no confirmation here: incus-compose
// asks on the terminal runSubprocess hands it, and refuses while running.
func (gui *Gui) backupRestore(backup *commands.ComposeBackup) error {
	target, ok := gui.selectedStackTarget()
	if !ok {
		return nil
	}

	items := []*types.MenuItem{{
		Label: fmt.Sprintf(gui.Tr.RestoreWholeStack, gui.onRemote(target.project, target.remote)),
		OnPress: func() error {
			return gui.backupRun(target, "restore", backup.Timestamp)
		},
	}}

	for _, service := range gui.backupServices() {
		items = append(items, &types.MenuItem{
			Label: fmt.Sprintf(gui.Tr.RestoreService, service.Name),
			OnPress: func() error {
				// The service after the timestamp, which composeRun puts last.
				serviceTarget := target
				serviceTarget.service = service.Name

				return gui.backupRun(serviceTarget, "restore", backup.Timestamp)
			},
		})
	}

	return gui.Menu(CreateMenuOptions{
		Title: fmt.Sprintf(gui.Tr.RestoreBackupMenuTitle, gui.backupLabel(backup)),
		Items: items,
	})
}

// backupServices are the stack's services with a named volume, the ones a
// restore can be narrowed to, the Services panel's selection first.
func (gui *Gui) backupServices() []*commands.ComposeService {
	services := lo.Filter(gui.composeServices(), func(service *commands.ComposeService, _ int) bool {
		return len(service.NamedVolumes()) > 0
	})

	selected, ok := gui.selectedService()
	if !ok {
		return services
	}

	slices.SortStableFunc(services, func(a, b *commands.ComposeService) int {
		return cmp.Compare(lo.Ternary(a.Name == selected.Name, 0, 1), lo.Ternary(b.Name == selected.Name, 0, 1))
	})

	return services
}

// backupVerify is `v`; its report is kept for the row and the Info tab.
func (gui *Gui) backupVerify(backup *commands.ComposeBackup) error {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return nil
	}

	key := gui.backupKey(backup)

	return gui.WithWaitingStatus(gui.Tr.VerifyingStatus, func() error {
		command, err := gui.commandFor(stack.Remote)
		if err != nil {
			return gui.createErrorPanel(err.Error())
		}

		verification, err := command.VerifyComposeBackup(stack.Dir, backup.Timestamp)
		if err != nil {
			return gui.createErrorPanel(err.Error())
		}

		gui.g.Update(func(*gocui.Gui) error {
			if gui.State.BackupVerifications == nil {
				gui.State.BackupVerifications = map[string]*commands.BackupVerification{}
			}

			gui.State.BackupVerifications[key] = verification

			if err := gui.Panels.Backups.RerenderList(); err != nil {
				return err
			}

			return gui.Panels.Backups.HandleSelect()
		})

		return nil
	})
}

// backupLabel is how a prompt names a backup: when it was taken, and its
// name if it has one.
func (gui *Gui) backupLabel(backup *commands.ComposeBackup) string {
	label := backup.CreatedAt().Local().Format(presentation.DateTimeFormat)
	if backup.Name != "" {
		label += " (" + backup.Name + ")"
	}

	return label
}

// handleGoToBackups focuses the Backups tab, whichever of its window's tabs
// was last on show: the selected stack's backups, one key from the stack.
// For a stack elsewhere it first moves the session to the stack's remote,
// as space does, the tab being there only for a stack on it.
func (gui *Gui) handleGoToBackups(g *gocui.Gui, v *gocui.View) error {
	focus := func() error {
		if gui.backupsAway() {
			return nil
		}

		gui.resetMainView()

		return gui.switchFocus(gui.Views.Backups)
	}

	stack, err := gui.Panels.Stacks.GetSelectedItem()
	if err != nil {
		return focus()
	}

	return gui.stackSwitchRemoteThen(stack, focus)
}
