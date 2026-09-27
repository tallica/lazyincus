package incustest

import (
	"encoding/json"
	"slices"
	"sync"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

// eventListener is commands.EventListener, spelled out: that package's
// tests import this one.
type eventListener = interface {
	AddHandler(types []string, function func(api.Event)) (*incus.EventTarget, error)
	Wait() error
	Disconnect()
}

// listener is one open event stream. project is empty for all projects.
type listener struct {
	shared  *state
	project string

	mutex    sync.Mutex
	handlers []handler

	done chan struct{}
	once sync.Once
	err  error
}

type handler struct {
	types    []string
	function func(api.Event)
}

func (l *listener) AddHandler(types []string, function func(api.Event)) (*incus.EventTarget, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.handlers = append(l.handlers, handler{types: types, function: function})

	return nil, nil
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

		l.err = err
		close(l.done)
	})
}

func (l *listener) send(event api.Event) {
	if l.project != "" && event.Project != l.project {
		return
	}

	l.mutex.Lock()
	handlers := slices.Clone(l.handlers)
	l.mutex.Unlock()

	for _, h := range handlers {
		if len(h.types) == 0 || slices.Contains(h.types, event.Type) {
			h.function(event)
		}
	}
}

// Listen opens an event stream that Emit feeds, standing in for
// GetEventsByType and GetEventsAllProjectsByType.
func (s *Server) Listen(allProjects bool, _ []string) (eventListener, error) {
	shared := s.shared()
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

// Listening is how many event streams are open.
func (s *Server) Listening() int {
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

// dropListeners ends every open stream the way a daemon going away does.
// Called with shared.mutex held.
func (shared *state) dropListeners() {
	open := shared.listeners
	shared.listeners = nil

	for _, l := range open {
		l.once.Do(func() {
			l.err = errUnreachable
			close(l.done)
		})
	}
}
