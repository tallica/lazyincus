package presentation

import (
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func TestOperationDuration(t *testing.T) {
	assert.Equal(t, "87ms", OperationDuration(87*time.Millisecond+400*time.Microsecond))
	assert.Equal(t, "1.5s", OperationDuration(1520*time.Millisecond))
	assert.Equal(t, "2m5s", OperationDuration(125*time.Second+300*time.Millisecond))
}

// operationRow is a row for an operation on an instance with a long name,
// which a narrow popup cuts short.
func operationRow(project, description string, status api.StatusCode, progress string) []string {
	return GetOperationDisplayStrings(&commands.Operation{Project: project, Operation: api.Operation{
		Description: description, Status: status.String(), StatusCode: status,
		Resources: map[string][]string{"instances": {"/1.0/instances/ic-seed-sblznjgurjoiypdy"}},
		Metadata:  map[string]any{"download_progress": progress},
	}}, true, false)
}

func renderOperations(t *testing.T, width int, rows ...[]string) []string {
	t.Helper()

	table, err := utils.RenderTableToWidth(rows, width, OperationFlexColumns(true, len("0s")))
	require.NoError(t, err)

	return strings.Split(utils.Decolorise(table), "\n")
}

// An operation under way saying how far it's got takes only the width the
// others leave, so their columns stay where they were.
func TestOperationProgressLeavesTheOtherColumnsBe(t *testing.T) {
	ended := operationRow("image-from-cache-nugvkmllqqxn", "Deleting instance", api.Success, "")
	running := operationRow("volume-prefetch-ibwdfghnklap", "Creating instance", api.Running,
		"Unpacking image: 100% (1.32kB/s)")

	for _, width := range []int{120, 90} {
		alone := renderOperations(t, width, ended)
		beside := renderOperations(t, width, running, ended)
		assert.Equal(t, alone[0], beside[1], "at %d columns", width)
	}
}

// Narrow, the instance's name is cut short rather than the row.
func TestOperationRowsFitANarrowPopup(t *testing.T) {
	rows := renderOperations(t, 55,
		operationRow("volume-prefetch-ibwdfghnklap", "Creating instance", api.Running, "Unpacking image"),
		operationRow("image-from-cache-nugvkmllqqxn", "Deleting instance", api.Success, ""))

	for _, row := range rows {
		assert.LessOrEqual(t, utils.DisplayWidth(row), 55, row)
		assert.Contains(t, row, "ic-se…")
	}
}
