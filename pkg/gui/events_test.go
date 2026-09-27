package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
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
		api.EventLifecycleInstanceUpdated,
		api.EventLifecycleImageRetrieved,
	} {
		assert.Zero(t, eventRefreshes[action], action)
	}
}

func TestAnInstanceComingOrGoingRefreshesEveryUsedByCount(t *testing.T) {
	assert.Equal(t, usedBy, eventRefreshes[api.EventLifecycleInstanceCreated])
	assert.Equal(t, refreshInstances, eventRefreshes[api.EventLifecycleInstanceStopped])
}

func TestABurstOfEventsIsOneRefresh(t *testing.T) {
	gui := &Gui{stopped: make(chan struct{})}
	close(gui.stopped) // flushEvents then only drains

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
