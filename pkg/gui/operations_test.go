package gui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	running, failed := log.counts(now.Add(-time.Second))
	assert.Equal(t, 1, running)
	assert.Equal(t, 1, failed)
}

// operationEvent is an operation event carrying what incustest.Operation
// leaves out: times, an error, whether it may be cancelled.
func operationEvent(t *testing.T, operation api.Operation) api.Event {
	t.Helper()

	metadata, err := json.Marshal(operation)
	require.NoError(t, err)

	return api.Event{Type: api.EventTypeOperation, Project: "default", Metadata: metadata}
}

func TestScreenOperations(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	download := api.Operation{
		ID: "download", Class: "task", Description: "Downloading image",
		Status: "Running", StatusCode: api.Running, MayCancel: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Resources: map[string][]string{"images": {"/1.0/images/0123456789ab"}},
		Metadata:  map[string]any{"download_progress": "rootfs: 23% (5.20MB/s)"},
	}
	// As the daemon would: running, it's listed as well as announced.
	s.server.SetOperations([]api.Operation{download})
	s.server.Emit(operationEvent(t, download))

	failed := api.Operation{
		ID: "start", Class: "task", Description: "Starting instance",
		Status: "Failure", StatusCode: api.Failure, Err: "no root device",
		CreatedAt: time.Now(), UpdatedAt: time.Now().Add(time.Second),
		Resources: map[string][]string{"instances": {"/1.0/instances/db"}},
	}
	s.server.Emit(operationEvent(t, failed))

	// Seen from anywhere: the footer counts both.
	screen := s.settle(t, "1 failed")
	assert.Contains(t, screen, "1 running")

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Operations) })

	screen = s.settle(t, "Downloading image")
	assert.Contains(t, screen, "✗ failure")

	// Under way sorts first.
	s.do(t, func() error {
		s.gui.Panels.Operations.SetSelectedLineIdx(0)
		return s.gui.Panels.Operations.HandleSelect()
	})
	s.settle(t, "Progress:     rootfs: 23% (5.20MB/s)")
	// Looked at, the failure stops being news.
	assert.NotContains(t, strings.Split(screen, "\n")[len(strings.Split(screen, "\n"))-2], "failed")

	// `d` cancels it, after asking.
	s.press(t, 'd')
	s.settle(t, "Are you sure you want to cancel Downloading image 0123456789ab?")
	s.pressKey(t, tcell.KeyEnter)

	assert.Eventually(t, func() bool {
		return lo.Contains(s.server.Cancelled(), "download")
	}, 5*time.Second, 20*time.Millisecond)
}

// The listing catches what started while no stream was open.
func TestOperationsListedAtStartup(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.server.SetOperations([]api.Operation{{
			ID: "export", Class: "task", Description: "Exporting instance", Status: "Running", StatusCode: api.Running,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Resources: map[string][]string{"instances": {"/1.0/instances/web?project=default"}},
		}})
	})
	s.ready(t)

	s.settle(t, "1 running")
	assert.Equal(t, []string{"export"}, onLoop(t, s, func() []string {
		return lo.Map(s.gui.Panels.Operations.List.GetAllItems(), func(operation *commands.Operation, _ int) string { return operation.Key() })
	}))
}

// A scope change clears the log before the old stream closes: what that
// stream still delivers belongs to the scope left behind.
func TestOperationsFromALeftScopeAreDropped(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	left := onLoop(t, s, func() uint64 { return s.gui.operationsScope.Add(1) - 1 })

	// Ended, so that no listing at startup takes them for gone.
	s.gui.recordOperation(left, testOperation("stale", api.Success, 0))
	s.gui.recordOperation(left+1, testOperation("current", api.Success, 0))

	assert.Eventually(t, func() bool {
		return slices.Equal([]string{"current"}, onLoop(t, s, func() []string {
			return lo.Map(s.gui.Panels.Operations.List.GetAllItems(), func(operation *commands.Operation, _ int) string { return operation.Key() })
		}))
	}, 5*time.Second, 20*time.Millisecond)
}
