package presentation

import (
	"fmt"

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
			owner, color.FgCyan))
	}

	return append(cells,
		// Local time, matching `incus info`; the API reports UTC.
		utils.ColoredString(snapshot.CreatedAt().Local().Format(DateTimeFormat), color.FgYellow),
		displaySnapshotExpiry(snapshot),
		displaySnapshotStateful(snapshot),
	)
}

// displaySnapshotExpiry marks the snapshots that delete themselves, since
// that's easy to forget having set. Blank for the ones that don't.
func displaySnapshotExpiry(snapshot *commands.Snapshot) string {
	if snapshot.ExpiresAt().IsZero() {
		return ""
	}

	return utils.ColoredString("expires "+snapshot.ExpiresAt().Local().Format(DateTimeFormat), color.FgMagenta)
}

// MinSnapshotNameWidth and MinSnapshotOwnerWidth are as narrow as the name
// and the instance it came from go for the columns after them.
const (
	MinSnapshotNameWidth  = 22
	MinSnapshotOwnerWidth = 22
)

func displaySnapshotStateful(snapshot *commands.Snapshot) string {
	if snapshot.IsStateful() {
		return utils.ColoredString("stateful", color.FgGreen)
	}

	return ""
}

// SnapshotConfig is the snapshot's Config tab, loc being what it was taken of.
func SnapshotConfig(snapshot *commands.Snapshot, loc Location) string {
	output := LocationLines(loc)
	if snapshot.Volume != nil {
		output += utils.WithPadding("Volume: ", IdentityPadding) + snapshot.Owner + " (" + snapshot.Volume.Pool + ")\n"
	} else {
		output += utils.WithPadding("Instance: ", IdentityPadding) + snapshot.Owner + "\n"
	}

	output += utils.WithPadding("Name: ", IdentityPadding) + snapshot.Name + "\n"
	output += utils.WithPadding("Taken at: ", IdentityPadding) + snapshot.CreatedAt().String() + "\n"

	if snapshot.Volume == nil {
		output += utils.WithPadding("Stateful: ", IdentityPadding) + fmt.Sprint(snapshot.IsStateful()) + "\n"
	}

	data, err := utils.MarshalIntoYaml(snapshot.Details())
	if err != nil {
		return fmt.Sprintf("Error marshalling snapshot details: %v", err)
	}

	return output + fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))
}
