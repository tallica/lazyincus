package presentation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
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

func ProfileDevices(profile *commands.Profile, tr *i18n.TranslationSet) string {
	if len(profile.Profile.Devices) == 0 {
		return tr.NoDevices
	}

	table, err := utils.RenderTable(GetDeviceRows(profile.Profile.Devices))
	if err != nil {
		return err.Error()
	}

	return table
}

func ProfileConfig(profile *commands.Profile, loc Location) string {
	output := LocationLines(loc)
	output += utils.WithPadding("Name: ", IdentityPadding) + profile.Name + "\n"
	output += utils.WithPadding("Used by: ", IdentityPadding) + fmt.Sprint(profile.UsedByCount()) + "\n"

	data, err := utils.MarshalIntoYaml(profile.Profile)
	if err != nil {
		return fmt.Sprintf("Error marshalling profile details: %v", err)
	}

	return output + fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))
}
