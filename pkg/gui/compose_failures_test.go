package gui

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func failedOperation(project, err string) commands.Event {
	return commands.Event{
		Type: api.EventTypeOperation, Project: project, Action: "Starting instance",
		Status: api.Failure, Instances: []string{"dns-server"},
		Details: &commands.Operation{Operation: api.Operation{StatusCode: api.Failure, Err: err}},
	}
}

func TestComposeFailuresKeepTheWatchedProjectsFailures(t *testing.T) {
	var failures composeFailures

	failures.note(failedOperation("dns", "before the watch"))
	failures.watch("dns")
	failures.note(failedOperation("other", "another project's"))
	failures.note(failedOperation("dns", "Failed to listen on 0.0.0.0:53"))

	succeeded := failedOperation("dns", "")
	succeeded.Status = api.Success
	failures.note(succeeded)

	assert.Equal(t, []string{"Starting instance dns-server: Failed to listen on 0.0.0.0:53"}, failures.stop())

	failures.note(failedOperation("dns", "after the watch"))
	assert.Empty(t, failures.stop())
}
