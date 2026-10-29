package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// listening opens m's event stream on a goroutine, as the program would,
// and takes the opening off the inbox.
func listening(t *testing.T, m *Model) {
	t.Helper()

	cmd := m.listen()
	go cmd()
	t.Cleanup(m.stopEvents)

	require.IsType(t, eventsOpenedMsg{}, deliver(t, m))
	flush(m)
}

// deliver hands m the next message off its inbox, and runs what it leads to.
func deliver(t *testing.T, m *Model) tea.Msg {
	t.Helper()

	select {
	case msg := <-m.inbox:
		_, cmd := m.Update(msg)
		run(m, cmd)
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("nothing came through the inbox")
		return nil
	}
}

func flush(m *Model) {
	run(m, func() tea.Msg { return flushEventsMsg{} })
}

func row(m *Model, name string) string {
	for _, line := range screen(m) {
		if strings.Contains(line, " "+name+" ") || strings.Contains(line, "│"+name+" ") {
			return line
		}
	}

	return ""
}

func eventServer() *incustest.Server {
	instance := func(name, status string) api.InstanceFull {
		return api.InstanceFull{Instance: api.Instance{Name: name, Project: "default", Status: status, Type: "container"}}
	}

	return incustest.New(incustest.Server{
		Version:   "7.4",
		Instances: []api.InstanceFull{instance("db", "Stopped"), instance("web", "Running")},
	})
}

func TestAnEventRefreshesItsListBeforeThePoll(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 90, 16)
	listening(t, m)

	server.SetInstances(append(server.Instances, api.InstanceFull{Instance: api.Instance{
		Name: "fresh", Project: "default", Status: "Running", Type: "container",
	}}))
	server.Emit(incustest.Lifecycle("default", api.EventLifecycleInstanceCreated, "/1.0/instances/fresh"))
	deliver(t, m)
	assert.Empty(t, row(m, "fresh"), "held for the batch window")

	flush(m)
	assert.NotEmpty(t, row(m, "fresh"))
}

func TestABurstOfEventsIsOneRefresh(t *testing.T) {
	m := newTestModel(t, options{})
	ticks := 0
	m.after = func(time.Duration, tea.Msg) tea.Cmd { ticks++; return nil }

	for _, action := range []string{
		api.EventLifecycleInstanceStarted, api.EventLifecycleImageDeleted, api.EventLifecycleInstanceConsoleRetrieved,
	} {
		m.Update(eventMsg{event: commands.Event{Type: api.EventTypeLifecycle, Action: action}})
	}

	assert.Equal(t, refreshInstances|refreshImages, m.events.pending)
	assert.Equal(t, 1, ticks)
}

func TestEventsFromAClosedStreamAreDropped(t *testing.T) {
	m := newTestModel(t, options{})
	m.events.stream = 2

	m.Update(eventMsg{stream: 1, event: commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleInstanceStarted}})
	assert.Zero(t, m.events.pending)
}

func TestAnOperationMarksItsInstance(t *testing.T) {
	for _, status := range []api.StatusCode{api.Success, api.Failure, api.Cancelled} {
		t.Run(status.String(), func(t *testing.T) {
			server := eventServer()
			m := start(t, options{server: server}, 90, 16)
			listening(t, m)

			server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Running, "web"))
			deliver(t, m)
			assert.Contains(t, row(m, "web"), "restarting")

			server.Emit(incustest.Operation("default", "op1", "Restarting instance", status, "web"))
			deliver(t, m)
			assert.Contains(t, row(m, "web"), "restarting", "until a listing after it lands")

			flush(m)
			assert.Contains(t, row(m, "web"), "running")
			assert.NotContains(t, row(m, "web"), "restarting")
		})
	}
}

func TestARunningAfterItsSuccessMarksNothing(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 90, 16)
	listening(t, m)

	server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Success, "web"))
	server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Running, "web"))
	deliver(t, m)
	deliver(t, m)
	flush(m)

	assert.NotContains(t, row(m, "web"), "restarting")
}

func TestADroppedStreamTakesItsMarksOffAndBacksOff(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 90, 16)
	var reopen tea.Msg
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd {
		if _, ok := msg.(reopenEventsMsg); ok {
			reopen = msg
		}
		return nil
	}
	listening(t, m)

	server.Emit(incustest.Operation("default", "op1", "Restarting instance", api.Running, "web"))
	deliver(t, m)
	require.Contains(t, row(m, "web"), "restarting")

	server.SetDown(true)
	require.IsType(t, eventsClosedMsg{}, deliver(t, m))
	assert.Equal(t, eventRetryMin, m.events.retry)
	assert.Equal(t, reopenEventsMsg{m.events.stream}, reopen)

	server.SetDown(false)
	flush(m)
	assert.NotContains(t, row(m, "web"), "restarting")
}

func TestTheRetryBacksOffWhileTheDaemonStaysAway(t *testing.T) {
	assert.Equal(t, eventRetryMin, nextRetry(0, 0))
	assert.Equal(t, 2*eventRetryMin, nextRetry(eventRetryMin, 0))
	assert.Equal(t, eventRetryMax, nextRetry(eventRetryMax, 0))
	assert.Equal(t, eventRetryMin, nextRetry(eventRetryMax, eventRetryMax+time.Second), "a stream that held starts again")
}

func TestInstanceUpdatedRefreshesUsedByAtMostEveryInterval(t *testing.T) {
	m := newTestModel(t, options{})
	var waits []time.Duration
	m.after = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if _, ok := msg.(instanceUpdatedMsg); ok {
			waits = append(waits, d)
		}
		return nil
	}
	updated := eventMsg{event: commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleInstanceUpdated}}

	m.Update(updated)
	assert.Equal(t, usedBy&^refreshImages, m.events.pending)
	assert.Empty(t, waits)

	m.Update(updated)
	m.Update(updated)
	require.Len(t, waits, 1, "one refresh waits out the interval for the run")
	assert.InDelta(t, updatedInterval, waits[0], float64(time.Second))

	m.Update(instanceUpdatedMsg{})
	assert.False(t, m.events.updateDue)
}
