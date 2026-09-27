package gui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// Events lazyincus's own polling makes the daemon send must not refresh
// anything, or each refresh would set off the next.
func TestEventsFromPollingRefreshNothing(t *testing.T) {
	for _, action := range []string{
		api.EventLifecycleInstanceConsoleRetrieved,
		api.EventLifecycleInstanceLogRetrieved,
		api.EventLifecycleInstanceExec,
		api.EventLifecycleImageRetrieved,
	} {
		assert.Zero(t, eventRefreshes[action], action)
	}
}

func TestAnInstanceComingOrGoingRefreshesEveryUsedByCount(t *testing.T) {
	assert.Equal(t, usedBy, eventRefreshes[api.EventLifecycleInstanceCreated])
	assert.Equal(t, refreshInstances, eventRefreshes[api.EventLifecycleInstanceStopped])
}

// A VM's agent reports its state once it's up; the list asks again then.
func TestAVMsAgentStartingRefreshesTheInstances(t *testing.T) {
	assert.Equal(t, refreshInstances, eventRefreshes[api.EventLifecycleInstanceAgentStarted])
}

func TestABurstOfEventsIsOneRefresh(t *testing.T) {
	gui := bareGui(t)

	gui.onEvent(commands.Event{Action: api.EventLifecycleInstanceStarted})
	gui.onEvent(commands.Event{Action: api.EventLifecycleImageDeleted})
	gui.onEvent(commands.Event{Action: api.EventLifecycleInstanceConsoleRetrieved})

	gui.events.mutex.Lock()
	assert.Equal(t, refreshInstances|refreshImages, gui.events.pending)
	assert.True(t, gui.events.scheduled)
	gui.events.mutex.Unlock()
}

// The images poll is ten seconds away; only the event can bring this in
// before settle gives up.
func TestAnEventRefreshesItsListBeforeThePoll(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	images := append(fixtureServer().Images, api.Image{
		Fingerprint: "fedcba9876543210fedcba9876543210", Project: "default", Type: "container",
		Properties: map[string]string{"description": "Freshly pulled"},
	})
	s.server.SetImages(images)
	s.server.Emit(incustest.Lifecycle("default", api.EventLifecycleImageCreated, "/1.0/images/fedcba98"))

	s.settle(t, "Freshly pulled")
}

func TestAProjectSwitchReopensTheEventStream(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.do(t, func() error { return s.gui.switchToProject("other") })
	s.settle(t, "(fake/other)")

	s.server.SetImages(append(fixtureServer().Images, api.Image{
		Fingerprint: "fedcba9876543210fedcba9876543210", Project: "other", Type: "container",
		Properties: map[string]string{"description": "Other project's"},
	}))

	// Keep sending until the reopened stream, scoped to "other", hears one.
	require.Eventually(t, func() bool {
		s.server.Emit(incustest.Lifecycle("other", api.EventLifecycleImageCreated, "/1.0/images/fedcba98"))
		return strings.Contains(s.snapshot(t), "Other project's")
	}, 5*time.Second, 100*time.Millisecond)
}

// The stream comes back after the daemon does, backing off meanwhile.
func TestTheEventStreamReopensAfterADrop(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetDown(true)
	require.Eventually(t, func() bool { return s.server.Listening() == 0 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetDown(false)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 10*time.Second, 50*time.Millisecond)
}

// A restart started anywhere reads restarting until it's done.
func TestAnOperationMarksItsInstance(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Running, "web"))
	require.Eventually(t, func() bool { return strings.Contains(row(s.snapshot(t), "web"), "restarting") },
		5*time.Second, 50*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Success, "web"))
	require.Eventually(t, func() bool { return strings.Contains(row(s.snapshot(t), "web"), "running") },
		5*time.Second, 50*time.Millisecond)
}

// row is the instances panel's line for the instance.
func row(screen, name string) string {
	for line := range strings.Lines(screen) {
		if strings.HasPrefix(line, "│"+name+" ") {
			return line
		}
	}

	return ""
}

// No event will say how an operation ends once the stream has gone.
func TestADroppedStreamTakesItsMarksOff(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Stopping instance", api.Running, "web"))
	s.settle(t, "stopping")

	s.server.SetDown(true)
	require.Eventually(t, func() bool { return !strings.Contains(s.snapshot(t), "stopping") }, 5*time.Second, 50*time.Millisecond)
}

func TestAnOperationWithNoRowStatusMarksNothing(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Executing command", api.Running, "web"))
	s.server.Emit(incustest.Operation("default", "op2", "Restarting instance", api.Running, "web"))
	s.settle(t, "restarting")

	s.do(t, func() error {
		s.gui.events.mutex.Lock()
		defer s.gui.events.mutex.Unlock()
		assert.Len(t, s.gui.events.operations, 1)
		return nil
	})
}

// Nothing said what changed while the stream was down, so it catches up
// when it reopens, rather than when the images poll comes round.
func TestAReopenedStreamCatchesUp(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetDown(true)
	require.Eventually(t, func() bool { return s.server.Listening() == 0 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetImages(append(fixtureServer().Images, api.Image{
		Fingerprint: "fedcba9876543210fedcba9876543210", Project: "default", Type: "container",
		Properties: map[string]string{"description": "Pulled while away"},
	}))
	s.server.SetDown(false)

	s.settle(t, "Pulled while away")
}

func TestAWatchedListIsPolledLessWhileTheStreamIsOpen(t *testing.T) {
	polls := func(live bool) int {
		gui := &Gui{stopped: make(chan struct{})}
		defer close(gui.stopped)

		gui.eventsLive.Store(live)

		var mutex sync.Mutex
		count := 0
		gui.pollUnlessWatched(5*time.Millisecond, func() error {
			mutex.Lock()
			defer mutex.Unlock()
			count++
			return nil
		})

		time.Sleep(100 * time.Millisecond)

		mutex.Lock()
		defer mutex.Unlock()

		return count
	}

	assert.Equal(t, 1, polls(true), "only the first poll, at startup")
	assert.Greater(t, polls(false), 1)
}

// An instance-updated refreshes the used-by lists straight away, and one
// right after it waits out the interval rather than refreshing again.
func TestInstanceUpdatedRefreshesUsedByAtMostEveryInterval(t *testing.T) {
	gui := bareGui(t)

	updated := commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleInstanceUpdated}

	gui.onEvent(updated)

	gui.events.mutex.Lock()
	assert.Equal(t, refreshVolumes|refreshNetworks|refreshProfiles, gui.events.pending)
	gui.events.pending = 0
	gui.events.mutex.Unlock()

	gui.onEvent(updated)
	gui.onEvent(updated)

	gui.events.mutex.Lock()
	defer gui.events.mutex.Unlock()
	assert.Zero(t, gui.events.pending, "within the interval")
	assert.True(t, gui.events.updateDue, "one more refresh, at the interval's end")
}

func TestTheRetryBacksOffWhileTheDaemonStaysAway(t *testing.T) {
	waits := make([]time.Duration, 0, 7)

	wait := time.Duration(0)
	for range 7 {
		wait = nextRetry(wait, 0)
		waits = append(waits, wait)
	}

	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 30 * time.Second, 30 * time.Second,
	}, waits)
}

func TestAStreamThatHeldStartsTheRetryAgain(t *testing.T) {
	assert.Equal(t, eventRetryMin, nextRetry(16*time.Second, time.Minute))
}

// Events arriving while a subprocess has the terminal wait for it, rather
// than being dropped.
func TestEventsWaitOutASubprocess(t *testing.T) {
	gui := bareGui(t)

	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleImageCreated})

	time.Sleep(3 * eventBatchWindow)

	gui.events.mutex.Lock()
	defer gui.events.mutex.Unlock()
	assert.Equal(t, refreshImages, gui.events.pending)
	assert.True(t, gui.events.scheduled)
}

// Names repeat across projects; an operation marks its own project's.
func TestAnOperationMarksOnlyItsProjectsInstance(t *testing.T) {
	s := startScreen(t, 160, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.SetInstances(append(fixtureServer().Instances, api.InstanceFull{Instance: api.Instance{
		Name: "web", Project: "other", Status: "Running", Type: "container",
	}}))
	require.NoError(t, s.gui.refreshInstances())
	s.settle(t, "│other ")

	s.server.Emit(incustest.Operation("other", "op1", "Stopping instance", api.Running, "web"))

	require.Eventually(t, func() bool {
		return strings.Contains(projectRow(s.snapshot(t), "other", "web"), "stopping")
	}, 5*time.Second, 50*time.Millisecond)
	defaultWeb := projectRow(s.snapshot(t), "default", "web")
	require.NotEmpty(t, defaultWeb)
	assert.NotContains(t, defaultWeb, "stopping")
}

// projectRow is the all-projects instances panel's line for the instance.
func projectRow(screen, project, name string) string {
	for line := range strings.Lines(screen) {
		if fields := strings.Fields(strings.TrimPrefix(line, "│")); len(fields) > 1 && fields[0] == project && fields[1] == name {
			return line
		}
	}

	return ""
}

// bareGui is a Gui with no screen or daemon, for what events queue. Paused,
// its flushes only queue again rather than refresh, which would need both.
// The test ends it, then waits out any flush already running, whose
// deadlock mutex reads the options the next test's NewGui writes.
func bareGui(t *testing.T) *Gui {
	t.Helper()

	gui := &Gui{stopped: make(chan struct{}), Log: commands.NewDummyLog()}
	gui.PauseBackgroundThreads.Store(true)
	t.Cleanup(func() {
		close(gui.stopped)
		time.Sleep(2 * eventBatchWindow)
	})

	return gui
}

func TestEveryEventIsLoggedUnderDebug(t *testing.T) {
	gui := bareGui(t)

	logger, hook := logtest.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)
	gui.Log = logrus.NewEntry(logger)

	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Project: "default", Action: api.EventLifecycleInstanceExec})
	gui.onEvent(commands.Event{
		Type: api.EventTypeOperation, Project: "default", Action: "Executing command",
		Operation: "op1", Status: api.Running, Instances: []string{"web"},
	})

	entries := hook.AllEntries()
	require.Len(t, entries, 2)
	assert.Equal(t, api.EventLifecycleInstanceExec, entries[0].Data["action"], "logged though it refreshes nothing")
	assert.Equal(t, "Running", entries[1].Data["status"])
	assert.Equal(t, []string{"web"}, entries[1].Data["instances"])
}

func TestEventsAreNotLoggedWithoutDebug(t *testing.T) {
	gui := bareGui(t)

	logger, hook := logtest.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	gui.Log = logrus.NewEntry(logger)

	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleInstanceExec})

	assert.Empty(t, hook.AllEntries())
}
