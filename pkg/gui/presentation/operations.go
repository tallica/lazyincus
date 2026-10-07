package presentation

import (
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// TimeFormat is DateTimeFormat's time of day alone, which an operation from
// this session needs.
const TimeFormat = "15:04:05"

// MinOperationDescriptionWidth is as narrow as the description goes for the
// columns after it.
const MinOperationDescriptionWidth = 18

// GetOperationDisplayStrings leads with the status, which is what the list
// is read for.
func GetOperationDisplayStrings(operation *commands.Operation, showProject bool) []string {
	cells := []string{
		OperationStatus(operation),
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

// OperationStatus is the status with a mark that reads at a glance.
func OperationStatus(operation *commands.Operation) string {
	switch operation.Operation.StatusCode {
	case api.Success:
		return utils.ColoredString("✓ "+operation.Status(), color.FgGreen)
	case api.Failure:
		return utils.ColoredString("✗ "+operation.Status(), color.FgRed)
	case api.Cancelled:
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

// OperationDuration rounds to what's worth reading: tenths under a minute,
// seconds above.
func OperationDuration(took time.Duration) string {
	if took < time.Minute {
		return took.Round(100 * time.Millisecond).String()
	}

	return took.Round(time.Second).String()
}
