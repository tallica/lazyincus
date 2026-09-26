package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// DateTimeFormat matches the "2025/04/16 23:45 CEST" style `incus info` uses
// for snapshot timestamps.
const DateTimeFormat = "2006/01/02 15:04 MST"

// DateFormat is DateTimeFormat's date alone.
const DateFormat = "2006/01/02"

// GetSnapshotDisplayStrings takes the name of the instance the snapshot came
// from, or "" when the title already says it.
func GetSnapshotDisplayStrings(snapshot *commands.Snapshot, owner string) []string {
	cells := []string{
		snapshot.Name,
	}

	if owner != "" {
		cells = append(cells, utils.ColoredString(
			utils.Truncate(owner, MinSnapshotNameWidth), color.FgCyan))
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

	return utils.ColoredString("expires "+snapshot.Snapshot.ExpiresAt.Local().Format(DateTimeFormat), color.FgMagenta)
}

// MinSnapshotNameWidth is as narrow as the name goes for the columns after
// it; the instance it came from keeps to that width outright.
const MinSnapshotNameWidth = 22

func displaySnapshotStateful(snapshot *commands.Snapshot) string {
	if snapshot.Snapshot.Stateful {
		return utils.ColoredString("stateful", color.FgGreen)
	}

	return ""
}
