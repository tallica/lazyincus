package presentation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
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

// displayVolumeUsers marks a custom volume nothing has attached, and one
// holding a stack's backups, which nothing attaches either.
func displayVolumeUsers(volume *commands.Volume) string {
	if volume.IsComposeBackup() {
		return utils.ColoredString("backup", color.FgBlue)
	}

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

// VolumeConfig is the volume's Config tab.
func VolumeConfig(volume *commands.Volume, loc Location, tr *i18n.TranslationSet) string {
	output := LocationLines(loc)
	size := DisplayVolumeUsage(volume)
	if size == "" {
		size = tr.VolumeSizeUnknown
	}

	usedBy := strings.Join(volume.Users(), ", ")
	switch {
	case volume.IsComposeBackup():
		usedBy = fmt.Sprintf(tr.BackupOf, volume.BackupOf())
	case usedBy == "":
		usedBy = tr.UsedByNothing
	}

	output += utils.WithPadding("Name: ", IdentityPadding) + volume.Name + "\n"
	output += utils.WithPadding("Pool: ", IdentityPadding) + volume.Pool + " (" + volume.PoolDriver + ")\n"
	if space := DisplayPoolSpace(volume); space != "" {
		output += utils.WithPadding("Pool space: ", IdentityPadding) + space + "\n"
	}
	output += utils.WithPadding("Type: ", IdentityPadding) + volume.Volume.Type + "\n"
	output += utils.WithPadding("Content type: ", IdentityPadding) + volume.Volume.ContentType + "\n"
	output += utils.WithPadding("Size: ", IdentityPadding) + size + "\n"
	output += utils.WithPadding("Used by: ", IdentityPadding) + usedBy + "\n"

	data, err := utils.MarshalIntoYaml(volume.Volume)
	if err != nil {
		return fmt.Sprintf("Error marshalling volume details: %v", err)
	}

	return output + fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))
}
