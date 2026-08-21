package gui

import (
	"strconv"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// dateTimeFormat matches the "2025/04/16 23:45 CEST" style `incus info`
// uses for snapshot timestamps.
const dateTimeFormat = "2006/01/02 15:04 MST"

// renderInstanceSnapshotsToMain is a static (non-polling) render, like the
// Config tab: snapshots don't change from second to second the way logs or
// resource usage do, so there's no need for a ticker here. Read-only for
// now - no create/restore/delete actions, since those need per-row
// selection and keybindings that don't fit this plain-text main-panel tab
// model (see CLAUDE.md's Instances panel section for the reasoning).
func (gui *Gui) renderInstanceSnapshotsToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceSnapshotsStr(instance) })
}

func (gui *Gui) instanceSnapshotsStr(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok {
		return gui.Tr.WaitingForInstanceInfo
	}

	if len(full.Snapshots) == 0 {
		return gui.Tr.NoSnapshots
	}

	// Matches incus info's own Snapshots table exactly (Name/Taken at/
	// Expires at/Stateful) - no Size column, since InstanceSnapshot.Size
	// comes back as -1 (unset) from GetInstanceFull; getting a real size
	// would need a separate per-snapshot request we're not making here.
	rows := [][]string{{"NAME", "TAKEN AT", "EXPIRES AT", "STATEFUL"}}
	for _, snap := range full.Snapshots {
		expiresAt := ""
		if !snap.ExpiresAt.IsZero() {
			expiresAt = snap.ExpiresAt.Format(dateTimeFormat)
		}

		rows = append(rows, []string{
			snap.Name,
			snap.CreatedAt.Format(dateTimeFormat),
			expiresAt,
			strconv.FormatBool(snap.Stateful),
		})
	}

	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}

	return table
}
