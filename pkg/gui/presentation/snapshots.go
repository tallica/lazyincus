package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// dateTimeFormat matches the "2025/04/16 23:45 CEST" style `incus info` uses
// for snapshot timestamps.
const dateTimeFormat = "2006/01/02 15:04 MST"

func GetSnapshotDisplayStrings(snapshot *commands.Snapshot) []string {
	return []string{
		utils.Truncate(snapshot.Name, maxSnapshotNameWidth),
		// Local time, matching `incus info`; the API reports UTC.
		utils.ColoredString(snapshot.Snapshot.CreatedAt.Local().Format(dateTimeFormat), color.FgYellow),
		displaySnapshotStateful(snapshot),
	}
}

const maxSnapshotNameWidth = 22

func displaySnapshotStateful(snapshot *commands.Snapshot) string {
	if snapshot.Snapshot.Stateful {
		return utils.ColoredString("stateful", color.FgGreen)
	}

	return ""
}
