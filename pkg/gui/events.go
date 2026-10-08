package gui

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/commands"
)

// refreshKind is a set of lists an event can have changed.
type refreshKind uint8

const (
	refreshInstances refreshKind = 1 << iota
	refreshImages
	refreshVolumes
	refreshNetworks
	refreshProfiles
)

// usedBy is every list that counts what uses it: an instance coming or
// going changes them all.
const usedBy = refreshInstances | refreshImages | refreshVolumes | refreshNetworks | refreshProfiles

// eventRefreshes is every lifecycle action that changes what a list shows,
// and which, named one by one: see docs/Incus.md, "Events", for why.
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

	api.EventLifecycleProfileCreated: refreshProfiles,
	api.EventLifecycleProfileDeleted: refreshProfiles,
	api.EventLifecycleProfileRenamed: refreshProfiles,
	api.EventLifecycleProfileUpdated: refreshProfiles,
}

// operationStatuses is the row status each operation that takes a while
// shows, by its description (docs/Incus.md, "Events").
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
)

// eventBatch is the lists events have asked for since the last refresh,
// and the marks each operation under way has put on its instances.
type eventBatch struct {
	mutex      deadlock.Mutex
	pending    refreshKind
	scheduled  bool
	operations map[string][]func()
	// finished is the latest operations to have ended, oldest first, so a
	// Running sent after its Success marks nothing. ending is their marks,
	// which the next flush ends once its listing lands.
	finished []string
	ending   []func()

	// updatedAt is when an instance-updated last refreshed the used-by
	// lists; updateDue, that another is waiting out updatedInterval.
	updatedAt time.Time
	updateDue bool
}

// watchEvents keeps a listener open for as long as the app runs, reopening
// it after a project switch and, backing off, after a drop.
func (gui *Gui) watchEvents() {
	var wait time.Duration

	for {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			select {
			case <-gui.stopped:
			case <-gui.eventsRescope:
			case <-ctx.Done():
			}
			cancel()
		}()

		opened := time.Now()
		err := gui.IncusCommand.ListenForEvents(ctx, eventTypes, gui.eventsOpened, gui.onEvent)
		gui.eventsLive.Store(false)
		cancel()

		if gui.isStopped() {
			return
		}

		// No event will say how the operations under way end.
		gui.endOperations(gui.takeOperations(""))

		if err == nil {
			wait = 0
			continue
		}

		gui.Log.Warn("event stream: ", err)

		// A dropped stream may be the daemon gone: the instances' listing
		// is what says, and the popup needn't wait for the poll.
		gui.queueRefresh(refreshInstances)

		wait = nextRetry(wait, time.Since(opened))

		select {
		case <-gui.stopped:
			return
		case <-gui.eventsRescope:
		case <-time.After(wait):
		}
	}
}

// nextRetry is how long to wait before reopening a stream that lasted for
// lasted, the wait before it having been previous: doubling while the
// daemon stays away, back to the start after a stream that held.
func nextRetry(previous, lasted time.Duration) time.Duration {
	if previous == 0 || lasted > eventRetryMax {
		return eventRetryMin
	}

	return min(previous*2, eventRetryMax)
}

// eventsOpened catches up on whatever changed while no stream was open to
// say so, and tells the slowed pollers they can stay slow.
func (gui *Gui) eventsOpened() {
	if gui.isStopped() {
		return
	}

	gui.eventsLive.Store(true)
	gui.queueRefresh(usedBy)

	scope := gui.IncusCommand.ProjectName()
	if scope == "" {
		scope = "all projects"
	}

	gui.Log.WithField("scope", scope).Debug("event stream open")
}

// logEvent records every event the stream delivers, those that change
// nothing included, under --debug.
func (gui *Gui) logEvent(event commands.Event) {
	if !gui.Log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		return
	}

	fields := logrus.Fields{"type": event.Type, "project": event.Project, "action": event.Action}
	if event.Type == api.EventTypeOperation {
		fields["operation"] = event.Operation
		fields["status"] = event.Status.String()
		fields["instances"] = event.Instances
	}

	gui.Log.WithFields(fields).Debug("event")
}

// rescopeEvents has watchEvents reopen its listener for the scope the
// panels list now.
func (gui *Gui) rescopeEvents() {
	select {
	case gui.eventsRescope <- struct{}{}:
	default:
	}
}

func (gui *Gui) isStopped() bool {
	select {
	case <-gui.stopped:
		return true
	default:
		return false
	}
}

// onEvent runs on the listener's goroutine.
func (gui *Gui) onEvent(event commands.Event) {
	if gui.isStopped() {
		return
	}

	gui.logEvent(event)

	if event.Type == api.EventTypeOperation {
		gui.onOperation(event)
		return
	}

	if event.Action == api.EventLifecycleInstanceUpdated {
		gui.onInstanceUpdated()
		return
	}

	kinds := eventRefreshes[event.Action]
	if kinds == 0 {
		return
	}

	if strings.HasPrefix(event.Action, "network-forward-") || strings.HasPrefix(event.Action, "network-acl-") {
		gui.networkTabs.Add(1)
	}

	gui.queueRefresh(kinds)
}

// onInstanceUpdated refreshes the instances and services, whose tabs show
// an instance's config, and the lists counting what that config attaches
// it to - a volume, a network, a profile - at most once per
// updatedInterval, the last update in a run of them still getting its
// refresh.
func (gui *Gui) onInstanceUpdated() {
	gui.events.mutex.Lock()

	if gui.events.updateDue {
		gui.events.mutex.Unlock()
		return
	}

	gui.events.updateDue = true
	wait := time.Until(gui.events.updatedAt.Add(updatedInterval))
	gui.events.mutex.Unlock()

	refresh := func() {
		gui.events.mutex.Lock()
		gui.events.updateDue, gui.events.updatedAt = false, time.Now()
		gui.events.mutex.Unlock()

		gui.queueRefresh(usedBy &^ refreshImages)
	}

	if wait <= 0 {
		refresh()
		return
	}

	time.AfterFunc(wait, func() {
		if !gui.isStopped() {
			refresh()
		}
	})
}

// queueRefresh adds to the lists the next flush refreshes, scheduling one if
// none is.
func (gui *Gui) queueRefresh(kinds refreshKind) {
	gui.events.mutex.Lock()
	defer gui.events.mutex.Unlock()

	gui.events.pending |= kinds

	if !gui.events.scheduled {
		gui.events.scheduled = true
		time.AfterFunc(eventBatchWindow, gui.flushEvents)
	}
}

func (gui *Gui) flushEvents() {
	if gui.isStopped() {
		return
	}

	gui.events.mutex.Lock()
	kinds, ending := gui.events.pending, gui.events.ending
	gui.events.pending, gui.events.ending, gui.events.scheduled = 0, nil, false
	gui.events.mutex.Unlock()

	// A subprocess has the terminal; hold the lists until it's back.
	if gui.PauseBackgroundThreads.Load() {
		gui.events.mutex.Lock()
		gui.events.ending = append(ending, gui.events.ending...)
		gui.events.mutex.Unlock()

		gui.queueRefresh(kinds)

		return
	}

	if err := gui.refreshEnding(ending, gui.fetchesFor(kinds)...); err != nil {
		gui.Log.Warn(err)
	}

	// The listing just said whether the daemon answers - after a reopened
	// stream's catch-up, that it's back.
	if kinds&refreshInstances != 0 {
		gui.syncConnection()
	}
}

func (gui *Gui) fetchesFor(kinds refreshKind) []fetch {
	var fetches []fetch

	if kinds&refreshInstances != 0 {
		fetches = append(fetches, gui.fetchInstances, gui.fetchStacks, gui.fetchServices)
	}

	if kinds&refreshImages != 0 {
		fetches = append(fetches, gui.fetchImages)
	}

	// A backup is snapshots of volumes in a project of the stack's, so the
	// events that say one changed are the volumes'.
	if kinds&refreshVolumes != 0 {
		fetches = append(fetches, gui.fetchVolumes, gui.fetchBackups)
	}

	if kinds&refreshNetworks != 0 {
		fetches = append(fetches, gui.fetchNetworks)
	}

	if kinds&refreshProfiles != 0 {
		fetches = append(fetches, gui.fetchProfiles)
	}

	return fetches
}

// onOperation marks the instances an operation is under way on, whoever
// started it, the way inTransition does lazyincus's own.
func (gui *Gui) onOperation(event commands.Event) {
	status, ok := operationStatuses[event.Action]
	if !ok {
		return
	}

	if event.Status.IsFinal() {
		gui.endOperations(gui.finishOperation(event.Operation))
		return
	}

	if event.Status != api.Running {
		return
	}

	gui.events.mutex.Lock()

	// An operation reports Running again with each step of progress, and
	// can report it after its Success (docs/Incus.md, "Events").
	if _, marked := gui.events.operations[event.Operation]; marked || slices.Contains(gui.events.finished, event.Operation) {
		gui.events.mutex.Unlock()
		return
	}

	ends := make([]func(), 0, len(event.Instances))
	for _, name := range event.Instances {
		ends = append(ends, gui.IncusCommand.MarkInstance(event.Project, name, status))
	}

	if gui.events.operations == nil {
		gui.events.operations = map[string][]func(){}
	}

	gui.events.operations[event.Operation] = ends
	gui.events.mutex.Unlock()

	gui.g.Update(func(*gocui.Gui) error { return gui.rerenderInstanceLists() })
}

// finishedKept is how many ended operations finished remembers: a late
// Running trails its Success by moments, not by a hundred operations.
const finishedKept = 100

// finishOperation is takeOperations for an operation that has ended,
// remembering that it has.
func (gui *Gui) finishOperation(id string) []func() {
	gui.events.mutex.Lock()
	gui.events.finished = append(gui.events.finished, id)
	if len(gui.events.finished) > finishedKept {
		gui.events.finished = gui.events.finished[1:]
	}
	gui.events.mutex.Unlock()

	return gui.takeOperations(id)
}

// takeOperations forgets the operation's marks and returns them to end, or
// every operation's for an empty ID.
func (gui *Gui) takeOperations(id string) []func() {
	gui.events.mutex.Lock()
	defer gui.events.mutex.Unlock()

	if id != "" {
		ends := gui.events.operations[id]
		delete(gui.events.operations, id)

		return ends
	}

	var ends []func()
	for _, operation := range gui.events.operations {
		ends = append(ends, operation...)
	}

	gui.events.operations = nil

	return ends
}

// endOperations takes the marks off once a listing taken after the
// operations has applied - before it, the rows would fall back on a poll
// that may have caught them halfway. It goes through the batch, so a run
// of operations ending together is one refresh.
func (gui *Gui) endOperations(ends []func()) {
	if len(ends) == 0 {
		return
	}

	if gui.isStopped() {
		for _, end := range ends {
			end()
		}

		return
	}

	gui.events.mutex.Lock()
	gui.events.ending = append(gui.events.ending, ends...)
	gui.events.mutex.Unlock()

	gui.queueRefresh(refreshInstances)
}
