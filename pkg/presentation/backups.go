package presentation

import (
	"fmt"
	"strconv"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// BackupNameColumn gives way first when a row doesn't fit, down to
// MinBackupNameWidth: the verification after it is what the row is read for.
const (
	BackupNameColumn   = 1
	MinBackupNameWidth = 8
)

// GetBackupDisplayStrings leads with when the backup was taken, which is
// what names it to every backup verb; verification is nil until `v`, and
// comes before the pool, which is rarely more than one.
func GetBackupDisplayStrings(backup *commands.ComposeBackup, verification *commands.BackupVerification, tr *i18n.TranslationSet) []string {
	size := ""
	if backup.Size > 0 {
		size = units.GetByteSizeStringIEC(backup.Size, 2)
	}

	return []string{
		utils.ColoredString(backup.CreatedAt().Local().Format(DateTimeFormat), color.FgYellow),
		backup.Name,
		strconv.Itoa(len(backup.Volumes)) + " vol",
		utils.ColoredString(size, color.FgYellow),
		displayBackupVerification(verification, tr),
		utils.ColoredString(backup.Pool(), color.FgCyan),
	}
}

func displayBackupVerification(verification *commands.BackupVerification, tr *i18n.TranslationSet) string {
	if verification == nil {
		return ""
	}

	failed := 0
	for _, volume := range verification.Volumes {
		if volume.Status != commands.BackupVerifyOK {
			failed++
		}
	}

	if failed == 0 {
		return utils.ColoredString(tr.BackupVerifiedOK, color.FgGreen)
	}

	return utils.ColoredString(fmt.Sprintf(tr.BackupVerifyProblems, failed), color.FgRed)
}

// BackupVolumeStatus is `backup verify`'s word for a volume, blank before any.
func BackupVolumeStatus(status string) string {
	switch status {
	case "":
		return ""
	case commands.BackupVerifyOK:
		return utils.ColoredString(status, color.FgGreen)
	default:
		return utils.ColoredString(status, color.FgRed)
	}
}

// BackupInfo is the Info tab; a nil loc has no stack to place the backup in.
func BackupInfo(loc *Location, backup *commands.ComposeBackup, verification *commands.BackupVerification,
	width int, tr *i18n.TranslationSet,
) string {
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", IdentityPadding) + value + "\n"
	}

	output := ""
	if loc != nil {
		output += LocationLines(*loc)
	}

	output += line("Name", backup.Name)
	output += line("Taken at", backup.CreatedAt().Local().Format(DateTimeFormat))
	// What every incus-compose backup verb takes to name this one.
	output += line("Timestamp", backup.Timestamp)
	output += line("Pool", backup.Pool())

	if backup.Size > 0 {
		output += line("Size", units.GetByteSizeStringIEC(backup.Size, 2))
	}

	if verification == nil {
		output += line("Verified", tr.BackupNotVerified)
	}

	output += "\n" + SectionHeading(tr.VolumesTitle, width) + "\n\n"
	output += BackupVolumes(backup, verification)

	return output
}

// BackupVolumes pairs each volume with its backup and `verify`'s word on
// it; one the stack has gained since the backup has no backup side.
func BackupVolumes(backup *commands.ComposeBackup, verification *commands.BackupVerification) string {
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
		rows = append(rows, []string{volume.Source.Name, "→ " + volume.Backup.Project + "/" + volume.Backup.Name, BackupVolumeStatus(statuses[volume.Source.Name])})
	}

	if verification != nil {
		for _, volume := range verification.Volumes {
			if !listed[volume.Volume] {
				rows = append(rows, []string{volume.Volume, "", BackupVolumeStatus(volume.Status)})
			}
		}
	}

	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}

	return table
}

// BackupLabel is how a prompt names a backup: when it was taken, and its
// name if it has one.
func BackupLabel(backup *commands.ComposeBackup) string {
	label := backup.CreatedAt().Local().Format(DateTimeFormat)
	if backup.Name != "" {
		label += " (" + backup.Name + ")"
	}

	return label
}
