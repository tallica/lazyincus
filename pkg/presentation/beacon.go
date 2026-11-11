package presentation

import (
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// displayUpdate is the update column: ↻, ↑, then beacon's verdict, a rebuild sharing the ↑.
func displayUpdate(guiConfig *config.GuiConfig, changed, newerBuild bool, verdict commands.BeaconVerdict) string {
	markers := []string{displayChanged(guiConfig, changed)}

	if newerBuild || verdict.Status == commands.BeaconUpdateAvailable && verdict.Rebuilt() {
		markers = append(markers, utils.ColoredString("↑", color.FgYellow))
	}

	switch verdict.Status {
	case commands.BeaconUpdateAvailable:
		if !verdict.Rebuilt() {
			markers = append(markers, utils.ColoredString("↑"+verdict.Latest, color.FgYellow))
		}
	case commands.BeaconError:
		markers = append(markers, utils.ColoredString("!", color.FgRed))
	case commands.BeaconUnknown:
		markers = append(markers, utils.ColoredString("?", color.FgWhite))
	}

	return strings.Join(lo.Compact(markers), " ")
}

// BeaconLine is the Info tab's Update line, empty until beacon has checked.
func BeaconLine(verdict commands.BeaconVerdict) string {
	var what string

	switch verdict.Status {
	case "":
		return ""
	case commands.BeaconUpdateAvailable:
		what = verdict.Latest
	case commands.BeaconError, commands.BeaconUnknown:
		what = strings.TrimSpace(verdict.Status + ": " + verdict.Error)
		what = strings.TrimSuffix(what, ":")
	default:
		what = verdict.Status
	}

	source := "incus-beacon"
	if !verdict.Since.IsZero() {
		source += ", since " + LocalTime(verdict.Since)
	}

	return what + " (" + source + ")"
}
