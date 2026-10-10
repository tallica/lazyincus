package commands

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func TestOperationReadsItsResources(t *testing.T) {
	operation := &Operation{Operation: api.Operation{
		Resources: map[string][]string{
			"instances":           {"/1.0/instances/web?project=shop"},
			"instances_snapshots": {"/1.0/instances/web/snapshots/daily?project=shop"},
		},
		Metadata: map[string]any{
			"fs_progress":       "",
			"download_progress": "rootfs: 23% (5.20MB/s)",
		},
	}}

	assert.Equal(t, "web", operation.Target())
	assert.Equal(t, []string{"instances web", "instances_snapshots web/snapshots/daily"}, operation.Resources())
	// The first progress that says anything.
	assert.Equal(t, "rootfs: 23% (5.20MB/s)", operation.Progress())
	assert.Equal(t, "shop", operationProject(operation.Operation, "default"))
	assert.Equal(t, "default", operationProject(api.Operation{}, "default"))
}

func TestOperationTook(t *testing.T) {
	start := time.Date(2026, 10, 8, 0, 44, 0, 0, time.UTC)
	operation := &Operation{Operation: api.Operation{CreatedAt: start, UpdatedAt: start.Add(1500 * time.Millisecond), StatusCode: api.Running}}

	assert.Zero(t, operation.Took(), "still running")

	operation.Operation.StatusCode = api.Success
	assert.Equal(t, 1500*time.Millisecond, operation.Took(), "only ever listed: its last update")

	// The daemon leaves UpdatedAt alone when only the status changes.
	operation.Ended = start.Add(10 * time.Second)
	assert.Equal(t, 10*time.Second, operation.Took())
}

// A token runs for as long as it's valid, its secret in its metadata.
func TestGetOperationsLeavesOutTokens(t *testing.T) {
	server := incustest.New(incustest.Server{})
	server.SetOperations([]api.Operation{
		{ID: "task", Class: "task", Resources: map[string][]string{"instances": {"/1.0/instances/web?project=shop"}}},
		{ID: "token", Class: "token"},
	})

	operations, err := newEventsCommand(server).GetOperations()
	require.NoError(t, err)
	require.Len(t, operations, 1)
	assert.Equal(t, "task", operations[0].Key())
	assert.Equal(t, "shop", operations[0].Project)
}
