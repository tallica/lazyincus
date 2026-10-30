package tui

import (
	"encoding/json"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// operationEvent is pkg/gui's: an operation event carrying what incustest.Operation leaves out.
func operationEvent(t *testing.T, operation api.Operation) api.Event {
	t.Helper()

	metadata, err := json.Marshal(operation)
	require.NoError(t, err)

	return api.Event{Type: api.EventTypeOperation, Project: "default", Metadata: metadata}
}

func liveOperation(id string, status api.StatusCode) api.Operation {
	return api.Operation{
		ID: id, Class: "task", Description: "Starting instance", Status: status.String(), StatusCode: status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Resources: map[string][]string{"instances": {"/1.0/instances/db"}},
	}
}

func TestTheFooterCountsOperationsFromTheStream(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 140, 16)

	var ticks []tea.Msg
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd { ticks = append(ticks, msg); return nil }
	listening(t, m)

	server.Emit(operationEvent(t, liveOperation("start", api.Running)))
	deliver(t, m)
	server.Emit(operationEvent(t, liveOperation("failed", api.Failure)))
	deliver(t, m)

	assert.Contains(t, footer(m), "1 failed")
	assert.NotContains(t, footer(m), "running", "not under way for a second yet")
	assert.Contains(t, ticks, tea.Msg(operationsRecountMsg{}))

	// A second on, the recount shows it.
	for _, logged := range m.operations.entries {
		logged.first = logged.first.Add(-operationsShownAfter)
	}
	m.Update(operationsRecountMsg{})
	assert.Contains(t, footer(m), "1 failed 1 running")
}

// A stream left behind by a change of scope can still deliver; the log doesn't take it.
func TestAClosedStreamsOperationsAreDropped(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 140, 16)
	listening(t, m)

	stale := m.events.stream
	m.events.stream++

	operation := &commands.Operation{Operation: liveOperation("failed", api.Failure), Project: "default"}
	event := commands.Event{Type: api.EventTypeOperation, Details: operation}
	m.Update(eventMsg{stale, event})

	assert.Empty(t, m.operations.list())
	assert.NotContains(t, footer(m), "failed")

	m.Update(eventMsg{m.events.stream, event})
	assert.Contains(t, footer(m), "1 failed")
}

func TestOperationsUnderWayAreListedAtStartup(t *testing.T) {
	server := eventServer()
	server.SetOperations([]api.Operation{liveOperation("start", api.Running)})
	m := start(t, options{server: server}, 140, 16)

	require.Len(t, m.operations.list(), 1)
	assert.Equal(t, "start", m.operations.list()[0].Key())
}

func TestTheFooterCountsNewWarnings(t *testing.T) {
	server := eventServer()
	server.SetWarnings([]api.Warning{
		{UUID: "kvm", WarningPut: api.WarningPut{Status: "new"}, Severity: "low"},
		{UUID: "disk", WarningPut: api.WarningPut{Status: "new"}, Severity: "high"},
		{UUID: "dnsmasq", WarningPut: api.WarningPut{Status: "acknowledged"}, Severity: "low"},
	})
	m := start(t, options{server: server}, 140, 16)
	listening(t, m)

	assert.Contains(t, footer(m), "2 warnings")
	assert.Equal(t, "disk", m.warnings[0].Key())

	require.NoError(t, server.UpdateWarning("disk", api.WarningPut{Status: "acknowledged"}, ""))
	server.Emit(incustest.Lifecycle("", api.EventLifecycleWarningAcknowledged, "/1.0/warnings/disk"))
	deliver(t, m)
	flush(m)

	assert.Contains(t, footer(m), "1 warning ")
}

func TestWarningsArePolled(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 140, 16)
	assert.NotContains(t, footer(m), "warning")

	server.SetWarnings([]api.Warning{{UUID: "kvm", WarningPut: api.WarningPut{Status: "new"}}})
	run(m, m.pollWarnings())
	assert.Contains(t, footer(m), "1 warning ")
}

// The history is the scope's: a switch empties it.
func TestASwitchOfProjectClearsTheLog(t *testing.T) {
	m := start(t, options{server: eventServer()}, 140, 16)
	m.recordOperation(&commands.Operation{Operation: liveOperation("failed", api.Failure), Project: "default"})
	require.Contains(t, footer(m), "1 failed")

	run(m, m.switchToProject("default"))
	assert.Empty(t, m.operations.list())
	assert.NotContains(t, footer(m), "failed")
}

// A remote's warnings are its own: the footer counts the new one's.
func TestASwitchOfRemoteRecountsTheWarnings(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	pve01 := incustest.New(incustest.Server{Version: "7.5"})
	pve01.SetWarnings([]api.Warning{{UUID: "pve", WarningPut: api.WarningPut{Status: "new"}}})
	m := start(t, options{server: warningsServer(), compose: true, remotes: map[string]*incustest.Server{"pve01": pve01}}, 140, 40)
	require.Contains(t, footer(m), "2 warnings")

	run(m, m.switchToRemote("pve01"))
	assert.Contains(t, footer(m), "1 warning ")
	assert.Equal(t, []string{"pve"}, lo.Map(m.warnings, func(w *commands.Warning, _ int) string { return w.Key() }))
}

// A fetch asked of the scope left, landing after the switch, is turned away.
func TestAScopeChangeTurnsAwayOperationsAndWarningsInFlight(t *testing.T) {
	var seqs refreshSeqs
	operations, warnings := seqs.operations.issue(), seqs.warnings.issue()

	seqs.invalidateAll()
	assert.False(t, seqs.operations.admit(operations))
	assert.False(t, seqs.warnings.admit(warnings))
}
