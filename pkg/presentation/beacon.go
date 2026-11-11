package presentation

import (
	"strings"
	"time"

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
		markers = append(markers, newerMarker())
	}

	switch verdict.Status {
	case commands.BeaconUpdateAvailable:
		if !verdict.Rebuilt() {
			markers = append(markers, utils.ColoredString("↑"+shortNewerTag(verdict), color.FgYellow))
		}
	case commands.BeaconError:
		markers = append(markers, utils.ColoredString("!", color.FgRed))
	case commands.BeaconUnknown:
		markers = append(markers, utils.ColoredString("?", color.FgWhite))
	}

	return strings.Join(lo.Compact(markers), " ")
}

// shortNewerTag drops the variant both tags share: 1.31-alpine over 1.18-alpine is 1.31.
func shortNewerTag(verdict commands.BeaconVerdict) string {
	latest, current := verdict.Latest, verdict.CurrentTag()

	for i, r := range latest {
		variant := latest[i:]
		if i > 0 && (r == '-' || r == '_') && len(current) > len(variant) && strings.HasSuffix(current, variant) {
			return latest[:i]
		}
	}

	return latest
}

const newerNone = "none"

// newerBuildFound names the key that takes the build, where one does.
func newerBuildFound(key string) string {
	if key == "" {
		return "yes"
	}

	return "yes, " + key + " takes it"
}

// updateKey is the key that takes an instance's newer image.
func updateKey(instance *commands.Instance) string {
	switch {
	case instance.ComposeService() != "":
		return "G"
	case instance.IsOCI():
		return "U"
	}

	return ""
}

// BeaconLines are the Info tab's Newer build and Newer tag lines: docs/Panels.md, "incus-beacon".
func BeaconLines(verdict commands.BeaconVerdict, key string) (build, tag string) {
	var what string

	switch verdict.Status {
	case "":
		return "", ""
	case commands.BeaconUpdateAvailable:
		if verdict.Rebuilt() {
			return withBeaconSource(newerBuildFound(key), verdict), ""
		}

		return "", withBeaconSource(verdict.Latest, verdict)
	case commands.BeaconCurrent:
		what = newerNone
	case commands.BeaconError, commands.BeaconUnknown:
		what = strings.TrimSuffix(strings.TrimSpace(verdict.Status+": "+verdict.Error), ":")
	default:
		what = verdict.Status
	}

	if verdict.Semver {
		return "", withBeaconSource(what, verdict)
	}

	return withBeaconSource(what, verdict), ""
}

// newerBuildLine: either source finding one wins, as in the column and G; else g's answer.
func newerBuildLine(upstream commands.UpstreamStatus, verdict commands.BeaconVerdict, beaconLine string) string {
	switch {
	case upstream.State == commands.UpstreamNewer:
		return newerBuildFound("G")
	case verdict.Status == commands.BeaconUpdateAvailable && verdict.Rebuilt():
		return beaconLine
	case upstream.State == commands.UpstreamCurrent:
		return newerNone
	case upstream.State == commands.UpstreamFailed:
		return upstream.Err.Error()
	}

	return beaconLine
}

func withBeaconSource(what string, verdict commands.BeaconVerdict) string {
	source := "incus-beacon"
	if !verdict.Since.IsZero() {
		source += " since " + sinceTime(verdict.Since, time.Now())
	}

	return what + " (" + source + ")"
}

// sinceTime leaves today's date out.
func sinceTime(since, now time.Time) string {
	if since.Local().Format(time.DateOnly) == now.Local().Format(time.DateOnly) {
		return since.Local().Format("15:04 MST")
	}

	return LocalTime(since)
}
