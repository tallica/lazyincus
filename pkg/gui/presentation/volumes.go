package presentation

import (
	"strconv"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetVolumeDisplayStrings(volume *commands.Volume, showProject bool) []string {
	cells := []string{
		utils.Truncate(volume.Name, maxVolumeNameWidth),
		utils.ColoredString(volume.Pool, color.FgCyan),
		utils.ColoredString(volume.Volume.Type, color.FgMagenta),
		utils.ColoredString(strconv.Itoa(volume.UsedByCount()), color.FgYellow),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(volume.Volume.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// maxVolumeNameWidth keeps image-backed volumes, whose names are full
// fingerprints, from pushing the other columns off the panel.
const maxVolumeNameWidth = 24
