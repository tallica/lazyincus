package presentation

import (
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// TimeFormat is DateTimeFormat's time of day alone, with seconds, which an
// operation from this session needs; SecondsFormat is the whole of it.
const (
	TimeFormat    = "15:04:05"
	SecondsFormat = "2006/01/02 15:04:05 MST"
)

// MinOperationDescriptionWidth is as narrow as the description goes for the
// columns after it.
const MinOperationDescriptionWidth = 18

// GetOperationDisplayStrings leads with the status, which is what the list
// is read for; retried is a failure tried again since, and gone through.
func GetOperationDisplayStrings(operation *commands.Operation, showProject, retried bool) []string {
	cells := []string{
		OperationStatus(operation, retried),
		operation.Operation.Description,
		utils.ColoredString(operation.Target(), color.FgCyan),
		utils.ColoredString(operation.Operation.CreatedAt.Local().Format(TimeFormat), color.FgYellow),
		operationProgress(operation),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(operation.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// OperationStatus is the status with a mark that reads at a glance, a
// failure retried since greyed out: nothing is left failed.
func OperationStatus(operation *commands.Operation, retried bool) string {
	switch {
	case retried:
		return utils.ColoredString("✗ "+operation.Status()+", retried", color.FgHiBlack)
	case operation.Operation.StatusCode == api.Success:
		return utils.ColoredString("✓ "+operation.Status(), color.FgGreen)
	case operation.Operation.StatusCode == api.Failure:
		return utils.ColoredString("✗ "+operation.Status(), color.FgRed)
	case operation.Operation.StatusCode == api.Cancelled:
		return utils.ColoredString("✗ "+operation.Status(), color.FgMagenta)
	default:
		return utils.ColoredString("● "+operation.Status(), color.FgYellow)
	}
}

// operationProgress is how long an ended operation took, or how far one
// under way has got when it says.
func operationProgress(operation *commands.Operation) string {
	if operation.IsFinal() {
		return OperationDuration(operation.Took())
	}

	return operation.Progress()
}

// OperationDuration rounds to what's worth reading: milliseconds under a
// second, tenths under a minute, seconds above.
func OperationDuration(took time.Duration) string {
	if took < time.Second {
		return took.Round(time.Millisecond).String()
	}

	if took < time.Minute {
		return took.Round(100 * time.Millisecond).String()
	}

	return took.Round(time.Second).String()
}
