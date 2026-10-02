package commands

import (
	"context"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func TestParseEventReadsALifecycleAction(t *testing.T) {
	event, ok := parseEvent(incustest.Lifecycle("prod", "instance-started", "/1.0/instances/web"), "default")

	require.True(t, ok)
	assert.Equal(t, Event{Type: api.EventTypeLifecycle, Project: "prod", Action: "instance-started"}, event)
}

// A daemon without the event_project extension doesn't say; the listener's
// own project does.
func TestParseEventFallsBackToTheListenersProject(t *testing.T) {
	event, ok := parseEvent(incustest.Lifecycle("", "instance-started", "/1.0/instances/web"), "default")

	require.True(t, ok)
	assert.Equal(t, "default", event.Project)
}

func TestParseEventSkipsLogging(t *testing.T) {
	_, ok := parseEvent(api.Event{Type: api.EventTypeLogging, Metadata: []byte(`{}`)}, "default")

	assert.False(t, ok)
}

// listenInBackground runs ListenForEvents until the test ends, handing its
// events and its result to channels.
func listenInBackground(t *testing.T, command *IncusCommand, server *incustest.Server) (<-chan Event, <-chan error, context.CancelFunc) {
	t.Helper()

	events := make(chan Event, 10)
	result := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		result <- command.ListenForEvents(ctx, []string{api.EventTypeLifecycle}, func() {}, func(event Event) { events <- event })
	}()

	require.Eventually(t, func() bool { return server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)

	return events, result, cancel
}

func newEventsCommand(server *incustest.Server) *IncusCommand {
	log := NewDummyLog()

	return NewIncusCommandWithClient(log, nil, nil, nil, server, "fake")
}

func TestListenForEventsPassesEventsOnUntilCancelled(t *testing.T) {
	server := incustest.New(incustest.Server{})
	events, result, cancel := listenInBackground(t, newEventsCommand(server), server)

	server.Emit(incustest.Lifecycle("default", "instance-stopped", "/1.0/instances/web"))
	assert.Equal(t, "instance-stopped", (<-events).Action)

	cancel()
	require.NoError(t, <-result)
	assert.Zero(t, server.Listening())
}

func TestListenForEventsReportsADrop(t *testing.T) {
	server := incustest.New(incustest.Server{})
	_, result, _ := listenInBackground(t, newEventsCommand(server), server)

	server.SetDown(true)

	assert.Error(t, <-result)
}

// Scoped to one project, the stream carries only that project's events.
func TestListenForEventsFollowsTheProjectInScope(t *testing.T) {
	server := incustest.New(incustest.Server{})
	command := newEventsCommand(server)
	command.UseProject("prod")
	events, _, _ := listenInBackground(t, command, server)

	server.Emit(incustest.Lifecycle("default", "instance-stopped", "/1.0/instances/web"))
	server.Emit(incustest.Lifecycle("prod", "instance-started", "/1.0/instances/api"))

	event := <-events
	assert.Equal(t, "prod", event.Project)
	assert.Equal(t, "instance-started", event.Action)
}

func TestParseEventReadsAnOperationsInstances(t *testing.T) {
	raw := incustest.Operation("prod", "op1", "Restarting instance", api.Running, "web")
	raw.Metadata = []byte(`{"id":"op1","description":"Restarting instance","status_code":103,
		"resources":{"instances":["/1.0/instances/web?project=prod","/1.0/instances/web/snapshots/daily","/1.0/instances/my%20vm"]}}`)

	event, ok := parseEvent(raw, "default")

	require.True(t, ok)
	assert.Equal(t, Event{
		Type: api.EventTypeOperation, Project: "prod", Action: "Restarting instance",
		Operation: "op1", Status: api.Running, Instances: []string{"web", "my vm"},
	}, event)
}

func TestListenForEventsKeepsTheirOrder(t *testing.T) {
	server := incustest.New(incustest.Server{})
	events, _, _ := listenInBackground(t, newEventsCommand(server), server)

	actions := []string{"instance-created", "instance-started", "instance-stopped", "instance-deleted"}
	for _, action := range actions {
		server.Emit(incustest.Lifecycle("default", action, "/1.0/instances/web"))
	}

	for _, action := range actions {
		assert.Equal(t, action, (<-events).Action)
	}
}

// A dial to a daemon that doesn't answer can't be cancelled: ListenForEvents
// returns without it, and the stream the dial opens late is closed.
func TestListenForEventsDoesNotWaitOutADial(t *testing.T) {
	server := incustest.New(incustest.Server{})
	release := server.HoldListen()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		result <- newEventsCommand(server).ListenForEvents(ctx, []string{api.EventTypeLifecycle}, func() {}, func(Event) {})
	}()

	cancel()

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("ListenForEvents waited out the dial")
	}

	release()
	require.Eventually(t, func() bool { return server.Open() == 0 }, 5*time.Second, 10*time.Millisecond)
}
