package gui

import (
	"context"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sasha-s/go-deadlock"
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
}

var eventTypes = []string{api.EventTypeLifecycle, api.EventTypeOperation}

const (
	// eventBatchWindow gathers a burst - a compose stack coming up is one
	// event per instance - into one refresh.
	eventBatchWindow = 200 * time.Millisecond

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
}

// watchEvents keeps a listener open for as long as the app runs, reopening
// it after a project switch and, backing off, after a drop.
func (gui *Gui) watchEvents() {
	delay := eventRetryMin

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
		err := gui.IncusCommand.ListenForEvents(ctx, eventTypes, gui.onEvent)
		cancel()

		if gui.isStopped() {
			return
		}

		// No event will say how the operations under way end.
		gui.endOperations(gui.takeOperations(""))

		if err == nil {
			delay = eventRetryMin
			continue
		}

		gui.Log.Warn("event stream: ", err)

		if time.Since(opened) > eventRetryMax {
			delay = eventRetryMin
		}

		select {
		case <-gui.stopped:
			return
		case <-gui.eventsRescope:
		case <-time.After(delay):
		}

		delay = min(delay*2, eventRetryMax)
	}
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
	if event.Type == api.EventTypeOperation {
		gui.onOperation(event)
		return
	}

	kinds := eventRefreshes[event.Action]
	if kinds == 0 {
		return
	}

	gui.queueRefresh(kinds)
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
	kinds := gui.events.pending
	gui.events.pending, gui.events.scheduled = 0, false
	gui.events.mutex.Unlock()

	// A subprocess has the terminal; hold the lists until it's back.
	if gui.PauseBackgroundThreads.Load() {
		gui.queueRefresh(kinds)
		return
	}

	if err := gui.refresh(nil, gui.fetchesFor(kinds)...); err != nil {
		gui.Log.Warn(err)
	}
}

func (gui *Gui) fetchesFor(kinds refreshKind) []fetch {
	var fetches []fetch

	if kinds&refreshInstances != 0 {
		fetches = append(fetches, gui.fetchInstances, gui.fetchServices)
	}

	if kinds&refreshImages != 0 {
		fetches = append(fetches, gui.fetchImages)
	}

	if kinds&refreshVolumes != 0 {
		fetches = append(fetches, gui.fetchVolumes)
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
		gui.endOperations(gui.takeOperations(event.Operation))
		return
	}

	if event.Status != api.Running {
		return
	}

	gui.events.mutex.Lock()

	// An operation reports Running again with each step of progress.
	if _, marked := gui.events.operations[event.Operation]; marked {
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
// that may have caught them halfway.
func (gui *Gui) endOperations(ends []func()) {
	if len(ends) == 0 {
		return
	}

	ended := func() error {
		for _, end := range ends {
			end()
		}

		return gui.rerenderInstanceLists()
	}

	if gui.isStopped() {
		for _, end := range ends {
			end()
		}

		return
	}

	go func() {
		if err := gui.refresh(ended, gui.fetchInstances, gui.fetchServices); err != nil {
			gui.g.Update(func(*gocui.Gui) error { return ended() })
		}
	}()
}
