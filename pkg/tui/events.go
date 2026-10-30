package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// refreshKind is a set of lists an event can have changed.
type refreshKind uint16

const (
	refreshInstances refreshKind = 1 << iota
	refreshImages
	refreshVolumes
	refreshNetworks
	refreshProfiles
	refreshWarnings
	// refreshOperations is the stream opening: what changed while it was shut.
	refreshOperations
	refreshStacks
	refreshBackups
)

// usedBy is every list that counts what uses it.
const usedBy = refreshInstances | refreshImages | refreshVolumes | refreshNetworks | refreshProfiles

// eventRefreshes is pkg/gui's (docs/Incus.md, "Events").
var eventRefreshes = map[string]refreshKind{
	api.EventLifecycleInstanceCreated:         usedBy,
	api.EventLifecycleInstanceDeleted:         usedBy,
	api.EventLifecycleInstanceRenamed:         usedBy,
	api.EventLifecycleInstanceStarted:         refreshInstances,
	api.EventLifecycleInstanceStopped:         refreshInstances,
	api.EventLifecycleInstanceShutdown:        refreshInstances,
	api.EventLifecycleInstanceRestarted:       refreshInstances,
	api.EventLifecycleInstancePaused:          refreshInstances,
	api.EventLifecycleInstanceResumed:         refreshInstances,
	api.EventLifecycleInstanceRestored:        refreshInstances,
	api.EventLifecycleInstanceMigrated:        refreshInstances,
	api.EventLifecycleInstanceAgentStarted:    refreshInstances,
	api.EventLifecycleInstanceAgentStopped:    refreshInstances,
	api.EventLifecycleInstanceSnapshotCreated: refreshInstances,
	api.EventLifecycleInstanceSnapshotDeleted: refreshInstances,
	api.EventLifecycleInstanceSnapshotRenamed: refreshInstances,
	api.EventLifecycleInstanceSnapshotUpdated: refreshInstances,

	api.EventLifecycleImageCreated:      refreshImages,
	api.EventLifecycleImageDeleted:      refreshImages,
	api.EventLifecycleImageUpdated:      refreshImages,
	api.EventLifecycleImageRefreshed:    refreshImages,
	api.EventLifecycleImageAliasCreated: refreshImages,
	api.EventLifecycleImageAliasDeleted: refreshImages,
	api.EventLifecycleImageAliasRenamed: refreshImages,
	api.EventLifecycleImageAliasUpdated: refreshImages,

	api.EventLifecycleStorageVolumeCreated:         refreshVolumes,
	api.EventLifecycleStorageVolumeDeleted:         refreshVolumes,
	api.EventLifecycleStorageVolumeRenamed:         refreshVolumes,
	api.EventLifecycleStorageVolumeUpdated:         refreshVolumes,
	api.EventLifecycleStorageVolumeRestored:        refreshVolumes,
	api.EventLifecycleStorageVolumeSnapshotCreated: refreshVolumes,
	api.EventLifecycleStorageVolumeSnapshotDeleted: refreshVolumes,
	api.EventLifecycleStorageVolumeSnapshotRenamed: refreshVolumes,
	api.EventLifecycleStorageVolumeSnapshotUpdated: refreshVolumes,
	api.EventLifecycleStoragePoolCreated:           refreshVolumes,
	api.EventLifecycleStoragePoolDeleted:           refreshVolumes,
	api.EventLifecycleStoragePoolUpdated:           refreshVolumes,

	api.EventLifecycleNetworkCreated:        refreshNetworks,
	api.EventLifecycleNetworkDeleted:        refreshNetworks,
	api.EventLifecycleNetworkRenamed:        refreshNetworks,
	api.EventLifecycleNetworkUpdated:        refreshNetworks,
	api.EventLifecycleNetworkForwardCreated: refreshNetworks,
	api.EventLifecycleNetworkForwardDeleted: refreshNetworks,
	api.EventLifecycleNetworkForwardUpdated: refreshNetworks,
	api.EventLifecycleNetworkACLCreated:     refreshNetworks,
	api.EventLifecycleNetworkACLDeleted:     refreshNetworks,
	api.EventLifecycleNetworkACLRenamed:     refreshNetworks,
	api.EventLifecycleNetworkACLUpdated:     refreshNetworks,

	api.EventLifecycleWarningAcknowledged: refreshWarnings,
	api.EventLifecycleWarningReset:        refreshWarnings,
	api.EventLifecycleWarningDeleted:      refreshWarnings,

	api.EventLifecycleProfileCreated: refreshProfiles,
	api.EventLifecycleProfileDeleted: refreshProfiles,
	api.EventLifecycleProfileRenamed: refreshProfiles,
	api.EventLifecycleProfileUpdated: refreshProfiles,
}

// operationStatuses is each slow operation's row status (docs/Incus.md, "Events").
var operationStatuses = map[string]string{
	"Starting instance":   "Starting",
	"Stopping instance":   "Stopping",
	"Restarting instance": "Restarting",
	"Restoring snapshot":  "Restoring",
	"Freezing instance":   "Freezing",
	"Unfreezing instance": "Unfreezing",
}

var eventTypes = []string{api.EventTypeLifecycle, api.EventTypeOperation}

const (
	// eventBatchWindow gathers a burst - a compose stack coming up is one
	// event per instance - into one refresh.
	eventBatchWindow = 200 * time.Millisecond

	// updatedInterval spaces out the refreshes instance-updated asks for,
	// ic-healthd sending one per instance every time it records a
	// healthcheck.
	updatedInterval = 10 * time.Second

	eventRetryMin = time.Second
	eventRetryMax = 30 * time.Second

	// finishedKept: a late Running trails its Success by moments, not 100 operations.
	finishedKept = 100
)

// events is pkg/gui's eventBatch and the stream's state, without the mutex.
type events struct {
	// stream numbers the listeners, so a closed one's messages are dropped.
	stream uint64
	cancel context.CancelFunc
	retry  time.Duration

	pending   refreshKind
	scheduled bool
	// finished is the latest operations to end, so a late Running marks nothing.
	operations map[string][]func()
	finished   []string
	ending     []func()

	updatedAt time.Time
	updateDue bool
}

// received is a message off the inbox, which one receive at a time reads.
type received struct{ msg tea.Msg }

type eventsOpenedMsg struct{ stream uint64 }

type eventMsg struct {
	stream uint64
	event  commands.Event
}

type eventsClosedMsg struct {
	stream uint64
	err    error
	lasted time.Duration
}

type reopenEventsMsg struct{ stream uint64 }

type flushEventsMsg struct{}

type instanceUpdatedMsg struct{}

func (m *Model) receive() tea.Cmd {
	inbox := m.inbox

	return func() tea.Msg { return received{<-inbox} }
}

// listen replaces the listener, which passes the stream to the inbox.
func (m *Model) listen() tea.Cmd {
	m.stopEvents()

	m.events.stream++
	stream := m.events.stream
	ctx, cancel := context.WithCancel(context.Background())
	m.events.cancel = cancel

	incus, inbox := m.incus, m.inbox
	send := func(msg tea.Msg) {
		select {
		case inbox <- msg:
		case <-ctx.Done():
		}
	}

	return func() tea.Msg {
		opened := time.Now()
		err := incus.ListenForEvents(ctx, eventTypes, func() { send(eventsOpenedMsg{stream}) }, func(event commands.Event) {
			send(eventMsg{stream, event})
		})
		send(eventsClosedMsg{stream, err, time.Since(opened)})

		return nil
	}
}

func (m *Model) stopEvents() {
	if m.events.cancel != nil {
		m.events.cancel()
		m.events.cancel = nil
	}
}

func (m *Model) onEventsOpened(msg eventsOpenedMsg) tea.Cmd {
	if msg.stream != m.events.stream {
		return nil
	}

	m.incus.Log.Debug().Str("scope", m.scopeName()).Msg("event stream open")
	m.eventsLive = true

	return m.queueRefresh(usedBy | refreshOperations)
}

func (m *Model) scopeName() string {
	if name := m.incus.ProjectName(); name != "" {
		return name
	}

	return "all projects"
}

// onEventsClosed is the end of pkg/gui's watchEvents loop.
func (m *Model) onEventsClosed(msg eventsClosedMsg) tea.Cmd {
	if msg.stream != m.events.stream {
		return nil
	}

	m.eventsLive = false
	// No event will say how the operations under way end.
	cmd := m.endOperations(m.takeOperations(""))

	if msg.err == nil {
		m.events.retry = 0
		return tea.Batch(cmd, m.listen())
	}

	m.incus.Log.Warn().Err(msg.err).Msg("event stream")
	m.events.retry = nextRetry(m.events.retry, msg.lasted)

	// A drop may be the daemon gone, which the instances' listing tells.
	return tea.Batch(cmd, m.queueRefresh(refreshInstances), m.tick(m.events.retry, reopenEventsMsg{msg.stream}))
}

// nextRetry doubles while the daemon stays away, resetting after a stream that held.
func nextRetry(previous, lasted time.Duration) time.Duration {
	if previous == 0 || lasted > eventRetryMax {
		return eventRetryMin
	}

	return min(previous*2, eventRetryMax)
}

func (m *Model) onEvent(msg eventMsg) tea.Cmd {
	if msg.stream != m.events.stream {
		return nil
	}

	event := msg.event
	m.logEvent(event)

	if event.Type == api.EventTypeOperation {
		return m.onOperation(event)
	}

	if event.Action == api.EventLifecycleInstanceUpdated {
		return m.onInstanceUpdated()
	}

	if strings.HasPrefix(event.Action, "network-forward-") || strings.HasPrefix(event.Action, "network-acl-") {
		m.networkTabs++
	}

	return m.queueRefresh(eventRefreshes[event.Action])
}

// logEvent records every event under --debug, those that change nothing too.
func (m *Model) logEvent(event commands.Event) {
	entry := m.incus.Log.Debug().Str("type", event.Type).Str("project", event.Project).Str("action", event.Action)
	if event.Type == api.EventTypeOperation {
		entry = entry.Str("operation", event.Operation).Stringer("status", event.Status).Strs("instances", event.Instances)
	}

	entry.Msg("event")
}

// onInstanceUpdated refreshes at most once per updatedInterval, the last of a run too.
func (m *Model) onInstanceUpdated() tea.Cmd {
	if m.events.updateDue {
		return nil
	}

	m.events.updateDue = true
	if wait := time.Until(m.events.updatedAt.Add(updatedInterval)); wait > 0 {
		return m.tick(wait, instanceUpdatedMsg{})
	}

	return m.refreshUpdated()
}

func (m *Model) refreshUpdated() tea.Cmd {
	m.events.updateDue, m.events.updatedAt = false, time.Now()

	return m.queueRefresh(usedBy &^ refreshImages)
}

func (m *Model) queueRefresh(kinds refreshKind) tea.Cmd {
	if kinds == 0 {
		return nil
	}

	m.events.pending |= kinds
	if m.events.scheduled {
		return nil
	}

	m.events.scheduled = true

	return m.tick(eventBatchWindow, flushEventsMsg{})
}

func (m *Model) flushEvents() tea.Cmd {
	kinds, ending := m.events.pending, m.events.ending
	m.events.pending, m.events.ending, m.events.scheduled = 0, nil, false

	msg := refreshMsg{quiet: true, ending: ending}
	// After a reopened stream's catch-up, the listing says the daemon is back.
	if kinds&refreshInstances != 0 {
		msg.next = syncConnection
	}

	fetches := m.fetchesFor(kinds)

	return func() tea.Msg {
		msg.applies, msg.err = gather(fetches)

		return msg
	}
}

// fetchesFor is pkg/gui's, for the kinds ported so far.
func (m *Model) fetchesFor(kinds refreshKind) []fetch {
	var fetches []fetch

	if kinds&refreshInstances != 0 {
		fetches = append(fetches, m.instanceFetches()...)
	}

	if kinds&refreshStacks != 0 {
		fetches = append(fetches, m.fetchStacks())
	}

	if kinds&refreshImages != 0 {
		fetches = append(fetches, m.fetchImages())
	}

	if kinds&refreshVolumes != 0 {
		fetches = append(fetches, m.fetchVolumes())
	}

	// A backup is a volume.
	if kinds&(refreshVolumes|refreshBackups) != 0 {
		fetches = append(fetches, m.fetchBackups())
	}

	if kinds&refreshNetworks != 0 {
		fetches = append(fetches, m.fetchNetworks())
	}

	if kinds&refreshProfiles != 0 {
		fetches = append(fetches, m.fetchProfiles())
	}

	return fetches
}

// onOperation marks the instances an operation is under way on, whoever started it.
func (m *Model) onOperation(event commands.Event) tea.Cmd {
	status, ok := operationStatuses[event.Action]
	if !ok {
		return nil
	}

	if event.Status.IsFinal() {
		return m.endOperations(m.finishOperation(event.Operation))
	}

	if event.Status != api.Running {
		return nil
	}

	// An operation reports Running again with each step of progress, and
	// can report it after its Success (docs/Incus.md, "Events").
	if _, marked := m.events.operations[event.Operation]; marked || slices.Contains(m.events.finished, event.Operation) {
		return nil
	}

	ends := make([]func(), 0, len(event.Instances))
	for _, name := range event.Instances {
		ends = append(ends, m.incus.MarkInstance(event.Project, name, status))
	}

	if m.events.operations == nil {
		m.events.operations = map[string][]func(){}
	}
	m.events.operations[event.Operation] = ends
	m.rerenderInstances()

	return nil
}

func (m *Model) finishOperation(id string) []func() {
	m.events.finished = append(m.events.finished, id)
	if len(m.events.finished) > finishedKept {
		m.events.finished = m.events.finished[1:]
	}

	return m.takeOperations(id)
}

// takeOperations returns the operation's marks to end, every one's for "".
func (m *Model) takeOperations(id string) []func() {
	if id != "" {
		ends := m.events.operations[id]
		delete(m.events.operations, id)

		return ends
	}

	var ends []func()
	for _, operation := range m.events.operations {
		ends = append(ends, operation...)
	}
	m.events.operations = nil

	return ends
}

// endOperations takes the marks off once a listing taken after the
// operations has applied: before it, the rows would fall back on a poll
// that may have caught them halfway.
func (m *Model) endOperations(ends []func()) tea.Cmd {
	if len(ends) == 0 {
		return nil
	}

	m.events.ending = append(m.events.ending, ends...)

	return m.queueRefresh(refreshInstances)
}
