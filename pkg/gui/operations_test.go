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

	// Seen from anywhere: the footer counts both, which needn't arrive together.
	require.Eventually(t, func() bool {
		footer := s.snapshot(t)
		return strings.Contains(footer, "1 failed") && strings.Contains(footer, "1 running")
	}, 5*time.Second, 50*time.Millisecond)

	// W opens on the operations, under way first.
	s.press(t, 'W')
	screen := s.settle(t, "Operations - Warnings")
	assert.Contains(t, screen, "✗ failure")
	assert.Regexp(t, `● running\s+Downloading image[^\n]*\n[^\n]*✗ failure`, screen)

	// Looked at, the failure stops being news.
	screen = s.settle(t, "1 running")
	assert.NotContains(t, strings.Split(screen, "\n")[len(strings.Split(screen, "\n"))-2], "failed")

	// enter shows the rest; closing it goes back to the list.
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Progress:     rootfs: 23% (5.20MB/s)")
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "d: cancel, enter: details")

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
		return lo.Map(s.gui.operations.list(), func(operation *commands.Operation, _ int) string { return operation.Key() })
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
			return lo.Map(s.gui.operations.list(), func(operation *commands.Operation, _ int) string { return operation.Key() })
		}))
	}, 5*time.Second, 20*time.Millisecond)
}

// recordOperations logs ended operations, one a second from operationStart,
// each described as its description says, and waits until they're in.
func recordOperations(t *testing.T, s *screen, descriptions ...string) {
	t.Helper()

	scope := onLoop(t, s, func() uint64 { return s.gui.operationsScope.Load() })
	for i, description := range descriptions {
		operation := testOperation(fmt.Sprintf("op%02d", i), api.Success, 0)
		operation.Operation.Description = description
		operation.Operation.CreatedAt = operationStart.Add(time.Duration(i) * time.Second)
		s.gui.recordOperation(scope, operation)
	}

	require.Eventually(t, func() bool {
		return onLoop(t, s, func() int { return len(s.gui.operations.list()) }) == len(descriptions)
	}, 5*time.Second, 20*time.Millisecond)
}

func TestFCutsTheOperationsDownByStatus(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	recordOperations(t, s, "Starting instance")
	scope := onLoop(t, s, func() uint64 { return s.gui.operationsScope.Load() })
	failed := testOperation("failed", api.Failure, 0)
	failed.Operation.Description = "Stopping instance"
	s.gui.recordOperation(scope, failed)

	s.press(t, 'W')
	s.settle(t, "Stopping instance")

	s.press(t, 'f')
	screen := s.settle(t, "Operations (failed) - Warnings")
	assert.Contains(t, screen, "✗ failure")
	assert.NotContains(t, screen, "Starting instance")

	s.press(t, 'f')
	screen = s.settle(t, "Operations (succeeded) - Warnings")
	assert.Contains(t, screen, "Starting instance")
	assert.NotContains(t, screen, "Stopping instance")

	s.press(t, 'f')
	screen = s.settle(t, "Operations - Warnings")
	assert.Contains(t, screen, "Starting instance")
	assert.Contains(t, screen, "Stopping instance")
}

// The fuzzy filter would put the exact match first; the operations stay in
// time order.
func TestFilteringTheOperationsKeepsTheirOrder(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	recordOperations(t, s, "Deleting snapshot", "Snapshotting instance", "Starting instance")

	s.press(t, 'W')
	s.settle(t, "Starting instance")
	s.press(t, '/')
	s.typeText(t, "snapshot")

	screen := s.settle(t, "filter: snapshot")
	assert.NotContains(t, screen, "Starting instance")
	assert.Regexp(t, `Snapshotting instance[^\n]*\n[^\n]*Deleting snapshot`, screen)

	// esc leaves the filter, then the popup.
	s.pressKey(t, tcell.KeyEnter)
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "Starting instance")
	assert.True(t, onLoop(t, s, func() bool { return s.gui.daemonOpen(daemonOperations) }))
}

// On the first row the cursor follows the newest; further down, the rows on
// screen stay put as new ones arrive above them.
func TestOperationsStayPutAsNewOnesArrive(t *testing.T) {
	s := startScreen(t, 140, 30, nil)
	s.ready(t)

	recordOperations(t, s, slices.Repeat([]string{"Executing command"}, 40)...)
	scope := onLoop(t, s, func() uint64 { return s.gui.operationsScope.Load() })
	arrive := func(id string, at time.Duration) {
		operation := testOperation(id, api.Success, 0)
		operation.Operation.CreatedAt = operationStart.Add(at)
		s.gui.recordOperation(scope, operation)
	}
	// The selected row, the first on screen, and the cursor's on screen.
	position := func() []int {
		return onLoop(t, s, func() []int {
			_, oy := s.gui.Views.Menu.Origin()
			_, cy := s.gui.Views.Menu.Cursor()
			return []int{s.gui.Panels.Menu.SelectedIdx, oy, cy}
		})
	}
	selected := func() string {
		return onLoop(t, s, func() string {
			item, _ := s.gui.Panels.Menu.GetSelectedItem()
			return item.Key
		})
	}

	s.press(t, 'W')
	s.settle(t, "Operations - Warnings")

	arrive("newest", time.Hour)
	require.Eventually(t, func() bool { return selected() == "newest" }, 5*time.Second, 20*time.Millisecond)

	for range 5 {
		s.pressKey(t, tcell.KeyDown)
	}
	require.Eventually(t, func() bool { return slices.Equal([]int{5, 0, 5}, position()) }, 5*time.Second, 20*time.Millisecond)
	reading := selected()

	arrive("newer still", 2*time.Hour)
	require.Eventually(t, func() bool { return slices.Equal([]int{6, 1, 5}, position()) }, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, reading, selected())
}
