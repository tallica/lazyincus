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

// GetBackupDisplayStrings leads with when the backup was taken, which is
// what names it to every backup verb; verification is nil until `v`.
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
		utils.ColoredString(backup.Pool(), color.FgCyan),
		displayBackupVerification(verification, tr),
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
