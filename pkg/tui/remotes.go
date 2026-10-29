package tui

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
)

// remoteRetryInterval is how long a remote that failed to connect is left
// before the next attempt: connecting can take connectTimeout.
const remoteRetryInterval = 30 * time.Second

// errConnecting is a remote whose connection is still being made, off to
// one side, so that nothing polling waits out connectTimeout for it.
var errConnecting = errors.New("connecting")

// remoteCommands is a command for each remote a stack is pinned to, other
// than the session's own, connected in the background the first time it's
// asked for.
type remoteCommands struct {
	mutex      sync.Mutex
	commands   map[string]*commands.IncusCommand
	failures   map[string]remoteFailure
	connecting map[string]bool

	// connect, known and names are the CLI config's; tests stand in a
	// second daemon here. connected is told when a connection made in the
	// background lands or fails.
	connect   func(name string) (*commands.IncusCommand, error)
	known     func(name string) bool
	names     func() []string
	connected func()

	// statuses are each remote's compose statuses as last read, read
	// again off to one side so the stacks never wait on another server.
	statuses        map[string]remoteStatuses
	readingStatuses map[string]bool
}

type remoteFailure struct {
	err error
	at  time.Time
}

type remoteStatuses struct {
	byProject map[string]commands.ComposeProjectState
	err       error
}

// commandFor is the command a stack pinned to remote goes through: the
// session's own for no remote, or for the session's remote by name. The
// first call for another remote starts connecting to it, and answers
// errConnecting until that's done.
func (m *Model) commandFor(remote string) (*commands.IncusCommand, error) {
	if m.onSessionRemote(remote) {
		return m.incus, nil
	}

	if !m.remotes.known(remote) {
		return nil, m.unknownRemote(remote)
	}

	return m.remotes.get(remote)
}

// unknownRemote is a stack's remote gone from the CLI's config, renamed or
// removed since the stack was saved, saying how to point the stack at
// another.
func (m *Model) unknownRemote(remote string) error {
	return fmt.Errorf(m.tr.UnknownRemote, remote)
}

// onSessionRemote is whether a stack pinned to remote is on the remote the
// rest of the panels show.
func (m *Model) onSessionRemote(remote string) bool {
	return remote == "" || remote == m.incus.RemoteName()
}

// stackRemote is the remote a stack is on, the session's when it names none.
func (m *Model) stackRemote(stack *commands.ComposeStack) string {
	if stack.Remote == "" {
		return m.incus.RemoteName()
	}

	return stack.Remote
}

// publishHostFor is where the ports of an instance on remote are reached,
// empty while that remote isn't connected.
func (m *Model) publishHostFor(remote string) string {
	if m.onSessionRemote(remote) {
		return m.incus.PublishHost()
	}

	m.remotes.mutex.Lock()
	defer m.remotes.mutex.Unlock()

	if command, ok := m.remotes.commands[remote]; ok {
		return command.PublishHost()
	}

	return ""
}

// remotesChangedMsg says a remote connected or its statuses changed.
type remotesChangedMsg struct{}

// remoteStatusErr is why another remote's statuses couldn't be read last
// time, without asking it: a remote known to be away is no reason to wait
// out a request to it.
func (r *remoteCommands) remoteStatusErr(remote string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.statuses[remote].err
}

// get is remote's command, or errConnecting while a connection is made
// in the background, or the last failure until remoteRetryInterval has
// passed. It never waits on the network.
func (r *remoteCommands) get(remote string) (*commands.IncusCommand, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if command, ok := r.commands[remote]; ok {
		return command, nil
	}

	if r.connecting[remote] {
		return nil, errConnecting
	}

	if failure, ok := r.failures[remote]; ok && time.Since(failure.at) < remoteRetryInterval {
		return nil, failure.err
	}

	if r.connecting == nil {
		r.connecting = map[string]bool{}
	}

	r.connecting[remote] = true

	go func() {
		command, err := r.connect(remote)
		r.store(remote, command, err)

		if r.connected != nil {
			r.connected()
		}
	}()

	return nil, errConnecting
}

func (r *remoteCommands) store(remote string, command *commands.IncusCommand, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.connecting, remote)

	if err != nil {
		if r.failures == nil {
			r.failures = map[string]remoteFailure{}
		}

		r.failures[remote] = remoteFailure{err: err, at: time.Now()}

		return
	}

	if r.commands == nil {
		r.commands = map[string]*commands.IncusCommand{}
	}

	delete(r.failures, remote)
	r.commands[remote] = command
}

// cachedStatuses is remote's compose statuses as last read, false when
// none has been yet, and starts another read unless one is under way.
// changed is told when that read finds something different.
func (r *remoteCommands) cachedStatuses(remote string, read func() (map[string]commands.ComposeProjectState, error), changed func()) (remoteStatuses, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	cached, ok := r.statuses[remote]

	if !r.readingStatuses[remote] {
		if r.readingStatuses == nil {
			r.readingStatuses = map[string]bool{}
		}

		r.readingStatuses[remote] = true

		go func() {
			byProject, err := read()

			r.mutex.Lock()
			previous, had := r.statuses[remote]
			delete(r.readingStatuses, remote)

			// Still connecting says nothing new: the connection's landing
			// asks again. Otherwise the answer is kept, but only statuses
			// that changed, or the remote starting or stopping answering,
			// refresh now: an error worded differently each time would
			// otherwise refresh, and so read again, without end. The next
			// poll shows the new wording.
			read := !errors.Is(err, errConnecting)
			changes := read && (!had || !reflect.DeepEqual(previous.byProject, byProject) || (previous.err == nil) != (err == nil))

			if read {
				if r.statuses == nil {
					r.statuses = map[string]remoteStatuses{}
				}

				r.statuses[remote] = remoteStatuses{byProject: byProject, err: err}
			}
			r.mutex.Unlock()

			if changes {
				changed()
			}
		}()
	}

	return cached, ok
}
