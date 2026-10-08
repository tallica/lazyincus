package presentation

import (
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// WarningSeverityRank orders the daemon's severities, an unknown one last.
func WarningSeverityRank(warning *commands.Warning) int {
	switch strings.ToLower(warning.Warning.Severity) {
	case "high":
		return 3
	case "moderate":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// GetWarningDisplayStrings leads with the severity, whether it's been
// acknowledged and how often it's been seen, and ends with the message,
// the one column that can stand being cut; when it was last seen is in its
// details.
func GetWarningDisplayStrings(warning *commands.Warning) []string {
	what := warning.Warning.LastMessage
	if entity := warning.Entity(); entity != "" {
		what = strings.TrimSpace(entity + " " + what)
	}

	return []string{
		displayWarningSeverity(warning),
		displayWarningStatus(warning),
		utils.ColoredString(strconv.Itoa(warning.Warning.Count)+"×", color.FgYellow),
		warning.Warning.Type,
		utils.ColoredString(warning.Warning.Project, color.FgCyan),
		what,
	}
}

func displayWarningSeverity(warning *commands.Warning) string {
	switch WarningSeverityRank(warning) {
	case 3:
		return utils.ColoredString(warning.Warning.Severity, color.FgRed)
	case 2:
		return utils.ColoredString(warning.Warning.Severity, color.FgYellow)
	default:
		return warning.Warning.Severity
	}
}

func displayWarningStatus(warning *commands.Warning) string {
	switch strings.ToLower(warning.Warning.Status) {
	case commands.WarningNew:
		return utils.ColoredString("new", color.FgYellow)
	case commands.WarningResolved:
		return utils.ColoredString("resolved", color.FgGreen)
	default:
		return "ack"
	}
}
