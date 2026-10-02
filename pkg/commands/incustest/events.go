package incustest

import (
	"encoding/json"
	"slices"
	"sync"

	"github.com/lxc/incus/v7/shared/api"
)

// eventListener is commands.EventListener, spelled out: that package's
// tests import this one.
type eventListener = interface {
	AddChannel(types []string, size int) <-chan api.Event
	Wait() error
	Disconnect()
}

// listener is one open event stream. project is empty for all projects.
type listener struct {
	shared  *state
	project string

	mutex    sync.Mutex
	channels []channel
	ended    bool

	done chan struct{}
	once sync.Once
	err  error
}

type channel struct {
	types []string
	ch    chan api.Event
}

// AddChannel is the client's: events in the order sent, the channel closed
// once the stream ends.
func (l *listener) AddChannel(types []string, size int) <-chan api.Event {
	if size <= 0 {
		size = 1000
	}

	ch := make(chan api.Event, size)

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.ended {
		close(ch)
		return ch
	}

	l.channels = append(l.channels, channel{types: types, ch: ch})

	return ch
}

func (l *listener) Wait() error {
	<-l.done
	return l.err
}

func (l *listener) Disconnect() {
	l.close(nil)
}

func (l *listener) close(err error) {
	l.once.Do(func() {
		l.shared.mutex.Lock()
		l.shared.listeners = slices.DeleteFunc(l.shared.listeners, func(open *listener) bool { return open == l })
		l.shared.mutex.Unlock()

		l.end(err)
	})
}

// end is close's second half, for a caller that has the listener out of
// the shared list already. Called once, under l.once.
func (l *listener) end(err error) {
	l.err = err
	close(l.done)

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.ended = true
	for _, c := range l.channels {
		close(c.ch)
	}

	l.channels = nil
}

func (l *listener) send(event api.Event) {
	if l.project != "" && event.Project != l.project {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	for _, c := range l.channels {
		if len(c.types) == 0 || slices.Contains(c.types, event.Type) {
			c.ch <- event
		}
	}
}

// Listen opens an event stream that Emit feeds, standing in for
// GetEventsByType and GetEventsAllProjectsByType.
func (s *Server) Listen(allProjects bool, _ []string) (eventListener, error) {
	shared := s.shared()

	shared.mutex.Lock()
	held, opened := shared.held, shared.heldOpened
	shared.held, shared.heldOpened = nil, nil
	shared.mutex.Unlock()

	if held != nil {
		<-held
		defer close(opened)
	}

	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	if shared.down {
		return nil, errUnreachable
	}

	l := &listener{shared: shared, done: make(chan struct{})}
	if !allProjects {
		l.project = s.scope()
	}

	shared.listeners = append(shared.listeners, l)

	return l, nil
}

// HoldListen has the next Listen wait until release, as a dial to a daemon
// that doesn't answer does. release returns once that Listen has opened
// its stream.
func (s *Server) HoldListen() (release func()) {
	held, opened := make(chan struct{}), make(chan struct{})

	shared := s.shared()
	shared.mutex.Lock()
	shared.held, shared.heldOpened = held, opened
	shared.mutex.Unlock()

	return func() {
		close(held)
		<-opened
	}
}

// Listening is how many event streams are open and being read: an event
// emitted before the client's AddChannel would be dropped, as a daemon's
// is, so a test waiting to emit waits for this rather than Open.
func (s *Server) Listening() int {
	shared := s.shared()
	shared.mutex.Lock()
	open := slices.Clone(shared.listeners)
	shared.mutex.Unlock()

	reading := 0

	for _, l := range open {
		l.mutex.Lock()
		if len(l.channels) > 0 {
			reading++
		}
		l.mutex.Unlock()
	}

	return reading
}

// Open is how many event streams are open, read or not.
func (s *Server) Open() int {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	return len(shared.listeners)
}

// Emit sends the event down every open stream it's in scope for.
func (s *Server) Emit(event api.Event) {
	shared := s.shared()
	shared.mutex.Lock()
	open := slices.Clone(shared.listeners)
	shared.mutex.Unlock()

	for _, l := range open {
		l.send(event)
	}
}

// Lifecycle is a lifecycle event, as the daemon sends one.
func Lifecycle(project, action, source string) api.Event {
	metadata, _ := json.Marshal(api.EventLifecycle{Action: action, Source: source})

	return api.Event{Type: api.EventTypeLifecycle, Project: project, Metadata: metadata}
}

// Operation is an operation event: the operation with the given ID, at
// status, acting on the instances.
func Operation(project, id, description string, status api.StatusCode, instances ...string) api.Event {
	paths := make([]string, len(instances))
	for i, name := range instances {
		paths[i] = "/1.0/instances/" + name
	}

	metadata, _ := json.Marshal(api.Operation{
		ID: id, Description: description, Status: status.String(), StatusCode: status,
		Resources: map[string][]string{"instances": paths},
	})

	return api.Event{Type: api.EventTypeOperation, Project: project, Metadata: metadata}
}

// dropListeners ends every open stream the way a daemon going away does.
// Called with shared.mutex held.
func (shared *state) dropListeners() {
	open := shared.listeners
	shared.listeners = nil

	for _, l := range open {
		l.once.Do(func() { l.end(errUnreachable) })
	}
}
