package gui

import (
	"errors"
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
	"github.com/tallica/lazyincus/pkg/i18n"
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
	require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "restarting") },
		5*time.Second, 50*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Success, "web"))
	require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "running") },
		5*time.Second, 50*time.Millisecond)
}

// An operation that fails or is cancelled is over too; nothing else would
// take its mark off.
func TestAnOperationThatDoesNotSucceedEndsItsMark(t *testing.T) {
	for _, status := range []api.StatusCode{api.Failure, api.Cancelled} {
		t.Run(status.String(), func(t *testing.T) {
			s := startScreen(t, 140, 40, nil)
			s.ready(t)
			require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

			s.server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Running, "web"))
			require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "restarting") },
				5*time.Second, 50*time.Millisecond)

			s.server.Emit(incustest.Operation("default", "op1", "Restarting instance", status, "web"))
			require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "running") },
				5*time.Second, 50*time.Millisecond)
		})
	}
}

// webRow is the instances panel's line for web, cut at the panel's edge:
// the main panel beside it shows the selected instance's status too.
func webRow(screen string) string {
	for line := range strings.Lines(screen) {
		if row, ok := strings.CutPrefix(line, "│web "); ok {
			row, _, _ = strings.Cut(row, "│")
			return row
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
	assert.Equal(t, refreshInstances|refreshVolumes|refreshNetworks|refreshProfiles, gui.events.pending)
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
// its flushes only queue again rather than refresh, which would need both,
// until the test ends it.
func bareGui(t *testing.T) *Gui {
	t.Helper()

	gui := &Gui{stopped: make(chan struct{}), Log: commands.NewDummyLog()}
	gui.PauseBackgroundThreads.Store(true)
	t.Cleanup(func() { close(gui.stopped) })

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

// The daemon can send an operation's Running after its Success; it mustn't
// leave a mark that nothing will end.
func TestARunningAfterItsSuccessMarksNothing(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Starting instance", api.Success, "web"))
	s.server.Emit(incustest.Operation("default", "op1", "Starting instance", api.Running, "web"))
	s.server.Emit(incustest.Operation("default", "op2", "Stopping instance", api.Running, "a-name-long-enough-to-be-cut-off"))
	s.settle(t, "stopping")

	assert.NotContains(t, webRow(s.snapshot(t)), "starting")
}

// Applying a fetch can fail after the fetch itself succeeded; refresh then
// skips its then, and the marks must come off anyway.
func TestMarksEndWhenApplyingTheRefreshFails(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	ended := make(chan struct{})
	failsToApply := func() (func() error, error) {
		return func() error { return errors.New("render failed") }, nil
	}

	require.NoError(t, s.gui.refreshEnding([]func(){func() { close(ended) }}, failsToApply))

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the mark was never ended")
	}
}

// Operations ending together share one refresh, and their marks wait out
// a subprocess along with it rather than being dropped.
func TestEndedOperationsAreBatched(t *testing.T) {
	gui := bareGui(t)

	gui.endOperations([]func(){func() {}})
	gui.endOperations([]func(){func() {}, func() {}})

	time.Sleep(3 * eventBatchWindow)

	gui.events.mutex.Lock()
	defer gui.events.mutex.Unlock()
	assert.Len(t, gui.events.ending, 3)
	assert.Equal(t, refreshInstances, gui.events.pending)
	assert.True(t, gui.events.scheduled)
}

// A pause from the CLI - or a whole service's incus-compose pause - marks
// each instance it freezes.
func TestAPauseFromAnywhereMarksItsInstance(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Freezing instance", api.Running, "web"))
	require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "freezing") },
		5*time.Second, 50*time.Millisecond)

	s.server.Emit(incustest.Operation("default", "op1", "Freezing instance", api.Success, "web"))
	s.server.Emit(incustest.Operation("default", "op2", "Unfreezing instance", api.Running, "web"))
	require.Eventually(t, func() bool { return strings.Contains(webRow(s.snapshot(t)), "unfreezing") },
		5*time.Second, 50*time.Millisecond)
}

// A tab is drawn once for the item it shows; a change a refresh brings
// draws it again, rather than waiting for the item to be selected anew.
func TestTheConfigTabFollowsAConfigChange(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.Panels.Instances.SetMainTab("config") })
	s.settle(t, "volatile.base_image")

	instances := fixtureServer().Instances
	for i := range instances {
		if instances[i].Name == "a-name-long-enough-to-be-cut-off" {
			instances[i].Config = map[string]string{
				"volatile.base_image": "0123456789abcdef0123456789abcdef", "user.note": "set-from-a-shell",
			}
		}
	}
	s.server.SetInstances(instances)
	require.NoError(t, s.gui.refreshInstances())

	s.settle(t, "set-from-a-shell")
}

// Forwards and ACLs aren't part of the network, so their events alone say
// its tabs are out of date.
func TestForwardAndACLEventsRedrawTheNetworkTabs(t *testing.T) {
	gui := bareGui(t)

	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleNetworkForwardCreated})
	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleNetworkACLUpdated})
	gui.onEvent(commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleNetworkUpdated})

	assert.Equal(t, uint64(2), gui.networkTabs.Load())
}

// The stream going and coming back is heard at once; the popup saying so
// needn't wait for the next two-second poll, either way.
func TestTheConnectionPopupFollowsTheStream(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	popup := func() bool { return strings.Contains(s.snapshot(t), s.gui.Tr.ConnectionLostTitle) }

	s.server.SetDown(true)
	require.Eventually(t, popup, 800*time.Millisecond, 20*time.Millisecond, "up after the drop")

	s.server.SetDown(false)
	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 10*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return !popup() }, 800*time.Millisecond, 20*time.Millisecond, "down after the reopen")
}

// Reading a tab means the main panel has focus, not the list; a change
// still has to reach it.
func TestTheConfigTabFollowsAChangeWhileBeingRead(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.Panels.Instances.SetMainTab("config") })
	s.settle(t, "volatile.base_image")
	s.do(t, func() error { return s.gui.handleEnterMain(s.g, s.gui.Views.Instances) })

	instances := fixtureServer().Instances
	for i := range instances {
		if instances[i].Name == "a-name-long-enough-to-be-cut-off" {
			instances[i].Config = map[string]string{
				"volatile.base_image": "0123456789abcdef0123456789abcdef", "environment.FOOBAR": "2",
			}
		}
	}
	s.server.SetInstances(instances)
	require.NoError(t, s.gui.refreshInstances())

	s.settle(t, "environment.FOOBAR")
	s.do(t, func() error {
		assert.True(t, s.gui.IsCurrentView(s.gui.Views.Main), "the main panel keeps focus")
		return nil
	})
}

// A service with one instance has no replica row: its own row's Config tab
// shows the instance's config, and has to follow a change to it.
func TestAServiceRowFollowsItsInstancesConfig(t *testing.T) {
	log := commands.NewDummyLog()
	gui := &Gui{Log: log, Tr: i18n.NewTranslationSet(log, "en")}
	key := gui.getServicesPanel().ContextState.GetItemContextCacheKey

	instance := &commands.Instance{Name: "redis-1", Project: "playground", Instance: api.InstanceFull{Instance: api.Instance{
		Status: "Running", InstancePut: api.InstancePut{Config: map[string]string{"environment.FOOBAR": "1"}},
	}}}
	row := &commands.ServiceRow{Service: &commands.ComposeService{Name: "redis", Instances: []*commands.Instance{instance}}}
	before := key(row)

	instance.Instance.Config = map[string]string{"environment.FOOBAR": "2"}

	assert.NotEqual(t, before, key(row))
}

// An image's, volume's, network's or profile's Config tab is drawn again
// when a refresh brings a change to it.
func TestAResourcesKeyFollowsItsConfig(t *testing.T) {
	log := commands.NewDummyLog()
	gui := &Gui{Log: log, Tr: i18n.NewTranslationSet(log, "en")}

	image := &commands.Image{Fingerprint: "abc"}
	imageKey := gui.getImagesPanel().ContextState.GetItemContextCacheKey
	before := imageKey(image)
	image.Image.Properties = map[string]string{"description": "changed"}
	assert.NotEqual(t, before, imageKey(image), "image")

	volume := &commands.Volume{Pool: "default", Name: "data"}
	volumeKey := gui.getVolumesPanel().ContextState.GetItemContextCacheKey
	before = volumeKey(volume)
	volume.Volume.Config = map[string]string{"size": "10GiB"}
	assert.NotEqual(t, before, volumeKey(volume), "volume")

	network := &commands.Network{Name: "incusbr0"}
	networkKey := gui.getNetworksPanel().ContextState.GetItemContextCacheKey
	before = networkKey(network)
	network.Network.Config = map[string]string{"ipv4.nat": "false"}
	assert.NotEqual(t, before, networkKey(network), "network")

	profile := &commands.Profile{Name: "default"}
	profileKey := gui.getProfilesPanel().ContextState.GetItemContextCacheKey
	before = profileKey(profile)
	profile.Profile.Config = map[string]string{"limits.cpu": "2"}
	assert.NotEqual(t, before, profileKey(profile), "profile")
}

// A snapshot's Config tab follows a change to it - an expiry set from a
// shell - an instance's snapshot and a volume's alike.
func TestASnapshotsKeyFollowsItsConfig(t *testing.T) {
	log := commands.NewDummyLog()
	gui := &Gui{Log: log, Tr: i18n.NewTranslationSet(log, "en")}
	key := gui.getSnapshotsPanel().ContextState.GetItemContextCacheKey

	ofInstance := &commands.Snapshot{Project: "default", Owner: "web", Name: "snap0"}
	before := key(ofInstance)
	ofInstance.Snapshot.ExpiresAt = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	assert.NotEqual(t, before, key(ofInstance), "instance's")

	ofVolume := &commands.Snapshot{Owner: "data", Name: "snap0", Volume: &commands.Volume{Pool: "default", Name: "data"}}
	before = key(ofVolume)
	ofVolume.VolumeSnapshot.ExpiresAt = new(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	assert.NotEqual(t, before, key(ofVolume), "volume's")
}

// A volume's usage changes with every write to it; a tab redrawn for each
// would never hold still.
func TestAVolumesKeyIgnoresItsUsage(t *testing.T) {
	log := commands.NewDummyLog()
	gui := &Gui{Log: log, Tr: i18n.NewTranslationSet(log, "en")}
	key := gui.getVolumesPanel().ContextState.GetItemContextCacheKey

	volume := &commands.Volume{Pool: "default", Name: "data", Usage: &api.StorageVolumeStateUsage{Used: 1024}}
	before := key(volume)

	volume.Usage = &api.StorageVolumeStateUsage{Used: 2048}

	assert.Equal(t, before, key(volume))
}
