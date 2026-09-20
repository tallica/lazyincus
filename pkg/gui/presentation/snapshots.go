package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// DateTimeFormat matches the "2025/04/16 23:45 CEST" style `incus info` uses
// for snapshot timestamps.
const DateTimeFormat = "2006/01/02 15:04 MST"

func GetSnapshotDisplayStrings(snapshot *commands.Snapshot, showInstance bool) []string {
	cells := []string{
		utils.Truncate(snapshot.Name, maxSnapshotNameWidth),
	}

	// Only when the panel is holding several instances' snapshots: on one
	// instance's the column would repeat the name already in the title.
	if showInstance {
		cells = append(cells, utils.ColoredString(
			utils.Truncate(snapshot.InstanceName, maxSnapshotNameWidth), color.FgCyan))
	}

	return append(cells,
		// Local time, matching `incus info`; the API reports UTC.
		utils.ColoredString(snapshot.Snapshot.CreatedAt.Local().Format(DateTimeFormat), color.FgYellow),
		displaySnapshotExpiry(snapshot),
		displaySnapshotStateful(snapshot),
	)
}

// displaySnapshotExpiry marks the snapshots that delete themselves, since
// that's easy to forget having set. Blank for the ones that don't.
func displaySnapshotExpiry(snapshot *commands.Snapshot) string {
	if snapshot.Snapshot.ExpiresAt.IsZero() {
		return ""
	}

	return utils.ColoredString("expires "+snapshot.Snapshot.ExpiresAt.Local().Format(DateTimeFormat), color.FgBlue)
}

const maxSnapshotNameWidth = 22

func displaySnapshotStateful(snapshot *commands.Snapshot) string {
	if snapshot.Snapshot.Stateful {
		return utils.ColoredString("stateful", color.FgGreen)
	}

	return ""
}
