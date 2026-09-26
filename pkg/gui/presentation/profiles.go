package presentation

import (
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// ProfileDescriptionColumn is where the description sits, the project
// column aside: the flexible column, and last, being the longest.
const ProfileDescriptionColumn = 3

// MinProfileDescriptionWidth is as narrow as the description goes.
const MinProfileDescriptionWidth = 16

// GetProfileDisplayStrings leads with the users count and the devices the
// profile hands out, which is what sets one profile apart from the next.
func GetProfileDisplayStrings(profile *commands.Profile, showProject bool) []string {
	devices := lo.Keys(profile.Profile.Devices)
	slices.Sort(devices)

	cells := []string{
		profile.Name,
		utils.ColoredString(strconv.Itoa(profile.UsedByCount()), color.FgYellow),
		utils.ColoredString(strings.Join(devices, " "), color.FgMagenta),
		profile.Profile.Description,
	}

	if showProject {
		cells = append([]string{utils.ColoredString(profile.Profile.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// GetDeviceRows lays out devices a row each, under a header: the name, the
// type, and the rest of its keys as key=value.
func GetDeviceRows(devices map[string]map[string]string) [][]string {
	names := lo.Keys(devices)
	slices.Sort(names)

	rows := make([][]string, 0, 1+len(names))
	rows = append(rows, []string{"NAME", "TYPE", "SETTINGS"})

	for _, name := range names {
		device := devices[name]

		keys := lo.Without(lo.Keys(device), "type")
		slices.Sort(keys)

		settings := lo.Map(keys, func(key string, _ int) string { return key + "=" + device[key] })

		rows = append(rows, []string{
			name,
			utils.ColoredString(device["type"], color.FgMagenta),
			strings.Join(settings, " "),
		})
	}

	return rows
}
