package presentation

import (
	"fmt"
	"strconv"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// GetVolumeDisplayStrings leads with the users count and the size, which
// is what a narrow panel keeps: whether the volume belongs to anything, and
// what it costs.
func GetVolumeDisplayStrings(volume *commands.Volume, showProject bool) []string {
	cells := []string{
		volume.Name,
		displayVolumeUsers(volume),
		utils.ColoredString(DisplayVolumeUsage(volume), color.FgYellow),
		utils.ColoredString(volume.Pool, color.FgCyan),
		utils.ColoredString(volume.Volume.Type, color.FgMagenta),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(volume.Volume.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// displayVolumeUsers marks a custom volume nothing has attached.
func displayVolumeUsers(volume *commands.Volume) string {
	count := strconv.Itoa(volume.UsedByCount())
	if volume.IsOrphaned() {
		return utils.ColoredString(count, color.FgRed)
	}

	return utils.ColoredString(count, color.FgGreen)
}

// DisplayVolumeUsage is blank where the pool's driver can't size a volume.
func DisplayVolumeUsage(volume *commands.Volume) string {
	if volume.Usage == nil {
		return ""
	}

	return units.GetByteSizeStringIEC(int64(volume.Usage.Used), 2)
}

// DisplayPoolSpace is a pool's use of its space, as `incus storage info`
// reports it.
func DisplayPoolSpace(volume *commands.Volume) string {
	space := volume.PoolSpace
	if space == nil || space.Total == 0 {
		return ""
	}

	return fmt.Sprintf("%s of %s used (%d%%)",
		units.GetByteSizeStringIEC(int64(space.Used), 2),
		units.GetByteSizeStringIEC(int64(space.Total), 2),
		space.Used*100/space.Total)
}

// MinVolumeNameWidth is as narrow as the name goes to make room for the
// columns after it: image-backed volumes are named by full fingerprint.
const MinVolumeNameWidth = 24
