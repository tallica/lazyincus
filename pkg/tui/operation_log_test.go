package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

var operationStart = time.Date(2026, 10, 8, 0, 44, 0, 0, time.UTC)

func testOperation(id string, status api.StatusCode, updated time.Duration) *commands.Operation {
	return &commands.Operation{Project: "default", Operation: api.Operation{
		ID: id, Class: "task", Description: "Snapshotting instance",
		Status: status.String(), StatusCode: status,
		CreatedAt: operationStart, UpdatedAt: operationStart.Add(updated),
		Resources: map[string][]string{"instances": {"/1.0/instances/web"}},
	}}
}

func logStatuses(log *operationLog) map[string]api.StatusCode {
	return lo.SliceToMap(log.list(), func(operation *commands.Operation) (string, api.StatusCode) {
		return operation.Key(), operation.Operation.StatusCode
	})
}

// The daemon can report Running after Success, and updates land in no
// particular order: an operation never goes back.
func TestOperationLogNeverGoesBack(t *testing.T) {
	var log operationLog
	now := time.Now()

	log.record(testOperation("op", api.Running, time.Second), now)
	log.record(testOperation("op", api.Success, 2*time.Second), now)
	log.record(testOperation("op", api.Running, 3*time.Second), now)
	log.record(testOperation("op", api.Pending, 0), now)

	assert.Equal(t, map[string]api.StatusCode{"op": api.Success}, logStatuses(&log))

	// As far along and older is a late update too.
	log.record(testOperation("ticking", api.Running, 2*time.Second), now)
	log.record(testOperation("ticking", api.Running, time.Second), now)
	assert.Equal(t, operationStart.Add(2*time.Second), lo.Must(lo.Find(log.list(), func(operation *commands.Operation) bool {
		return operation.Key() == "ticking"
	})).Operation.UpdatedAt)
}

// A listing of an operation that has ended doesn't say when; the event did.
func TestOperationLogKeepsWhenItEnded(t *testing.T) {
	var log operationLog
	now := time.Now()

	ended := testOperation("op", api.Failure, 0)
	ended.Ended = operationStart.Add(10 * time.Second)
	log.record(ended, now)
	log.record(testOperation("op", api.Failure, 0), now)

	assert.Equal(t, 10*time.Second, log.list()[0].Took())
}

func TestOperationLogLeavesOutTokens(t *testing.T) {
	var log operationLog

	token := testOperation("token", api.Running, 0)
	token.Operation.Class = "token"
	log.record(token, time.Now())

	assert.Empty(t, log.list())
}

// A listing is what's under way now. One the log has as under way that it
// doesn't list ended unseen and goes; one heard of after the listing was
// asked for is newer than it and stays; ended ones are the history.
func TestOperationLogReconcile(t *testing.T) {
	var log operationLog

	asked := time.Now()
	log.record(testOperation("ended-unseen", api.Running, 0), asked.Add(-time.Second))
	log.record(testOperation("started-since", api.Running, 0), asked.Add(time.Second))
	log.record(testOperation("history", api.Failure, time.Second), asked.Add(-time.Minute))

	log.reconcile([]*commands.Operation{testOperation("listed", api.Running, 0)}, asked, asked.Add(2*time.Second))

	assert.Equal(t, map[string]api.StatusCode{
		"started-since": api.Running,
		"history":       api.Failure,
		"listed":        api.Running,
	}, logStatuses(&log))
}

func TestOperationLogKeepsTheNewestEnded(t *testing.T) {
	var log operationLog
	now := time.Now()

	log.record(testOperation("running", api.Running, 0), now)
	for i := range operationsKept + 5 {
		log.record(testOperation(fmt.Sprint("ended-", i), api.Success, time.Duration(i)*time.Second), now)
	}

	statuses := logStatuses(&log)
	assert.Len(t, statuses, operationsKept+1)
	assert.Contains(t, statuses, "running")
	assert.Contains(t, statuses, fmt.Sprint("ended-", operationsKept+4))
	assert.NotContains(t, statuses, "ended-0")
}

func TestOperationLogCountsFailuresSince(t *testing.T) {
	var log operationLog
	now := time.Now()

	log.record(testOperation("running", api.Running, 0), now)
	log.record(testOperation("old-failure", api.Failure, 0), now.Add(-time.Minute))
	log.record(testOperation("new-failure", api.Failure, 0), now)
	log.record(testOperation("success", api.Success, 0), now)

	running, failed, due := log.counts(now.Add(-time.Second), now.Add(operationsShownAfter))
	assert.Equal(t, 1, running)
	assert.Equal(t, 1, failed)
	assert.True(t, due.IsZero())
}

// A failure tried again since, and gone through, leaves nothing failed:
// incus-compose retrying a delete the daemon refused while the instance was
// busy. Only the same action, on the same instance, started soon after,
// counts.
func TestOperationLogLeavesOutRetriedFailures(t *testing.T) {
	var log operationLog
	now := time.Now()

	at := func(op *commands.Operation, started time.Duration, instance string) *commands.Operation {
		op.Operation.Description = "Deleting instance"
		op.Operation.CreatedAt = operationStart.Add(started)
		op.Operation.Resources = map[string][]string{"instances": {"/1.0/instances/" + instance}}

		return op
	}

	log.record(at(testOperation("refused", api.Failure, 0), 0, "web-1"), now)
	log.record(at(testOperation("refused-again", api.Failure, 0), time.Second, "web-1"), now)
	log.record(at(testOperation("deleted", api.Success, 0), 2*time.Second, "web-1"), now)
	log.record(at(testOperation("refused-elsewhere", api.Failure, 0), 0, "web-2"), now)
	log.record(at(testOperation("deleted-before", api.Success, 0), -time.Second, "web-3"), now)
	log.record(at(testOperation("refused-after", api.Failure, 0), 0, "web-3"), now)
	log.record(at(testOperation("refused-long-ago", api.Failure, 0), 0, "web-4"), now)
	log.record(at(testOperation("deleted-by-hand", api.Success, 0), retriedWithin+time.Second, "web-4"), now)

	_, failed, _ := log.counts(now.Add(-time.Second), now)
	assert.Equal(t, 3, failed)

	retried := lo.FilterMap(log.list(), func(op *commands.Operation, _ int) (string, bool) {
		return op.Key(), log.retried(op)
	})
	assert.ElementsMatch(t, []string{"refused", "refused-again"}, retried)
}

func TestOperationLogCountsRunningOnceItHasLasted(t *testing.T) {
	var log operationLog
	now := time.Now()

	log.record(testOperation("running", api.Running, 0), now)

	running, _, due := log.counts(now, now.Add(operationsShownAfter/2))
	assert.Equal(t, 0, running)
	assert.Equal(t, now.Add(operationsShownAfter), due)

	// an update doesn't restart the wait
	log.record(testOperation("running", api.Running, 0), now.Add(operationsShownAfter/2))

	running, _, due = log.counts(now, now.Add(operationsShownAfter))
	assert.Equal(t, 1, running)
	assert.True(t, due.IsZero())
}
