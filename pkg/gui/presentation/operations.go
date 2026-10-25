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

// How far the columns narrow, in OperationFlexColumns' order.
const (
	MinOperationTargetWidth      = 12
	MinOperationProjectWidth     = 10
	MinOperationDescriptionWidth = 18

	NarrowOperationTargetWidth      = 6
	NarrowOperationProjectWidth     = 6
	NarrowOperationDescriptionWidth = 10
	NarrowOperationProgressWidth    = 5
)

// GetOperationDisplayStrings reads like a log: when, then the status.
func GetOperationDisplayStrings(operation *commands.Operation, showProject, retried bool) []string {
	cells := []string{
		utils.ColoredString(operation.Operation.CreatedAt.Local().Format(TimeFormat), color.FgYellow),
		OperationStatus(operation, retried),
	}

	if showProject {
		cells = append(cells, utils.ColoredString(operation.Project, color.FgCyan))
	}

	return append(cells,
		operation.Operation.Description,
		utils.ColoredString(operation.Target(), color.FgCyan),
		operationProgress(operation),
	)
}

// OperationFlexColumns narrows the progress first, to tookWidth: it can't shift the rest.
func OperationFlexColumns(showProject bool, tookWidth int) []utils.FlexColumn {
	// Without one, -1 for the project: RenderTableToWidth skips it.
	project, description := -1, 2
	if showProject {
		project, description = 2, 3
	}
	target, progress := description+1, description+2

	if tookWidth == 0 {
		tookWidth = NarrowOperationProgressWidth
	}

	return []utils.FlexColumn{
		{Index: progress, MinWidth: tookWidth},
		{Index: target, MinWidth: MinOperationTargetWidth},
		{Index: project, MinWidth: MinOperationProjectWidth},
		{Index: description, MinWidth: MinOperationDescriptionWidth},
		{Index: target, MinWidth: NarrowOperationTargetWidth},
		{Index: project, MinWidth: NarrowOperationProjectWidth},
		{Index: description, MinWidth: NarrowOperationDescriptionWidth},
		{Index: progress, MinWidth: NarrowOperationProgressWidth},
	}
}

// OperationStatus is the status with a mark that reads at a glance.
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

// OperationTookWidth is the width the widest duration of operations takes.
func OperationTookWidth(operations []*commands.Operation) int {
	width := 0
	for _, operation := range operations {
		if operation.IsFinal() {
			width = max(width, utils.DisplayWidth(OperationDuration(operation.Took())))
		}
	}

	return width
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
