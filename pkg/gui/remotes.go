package gui

import (
	"sync"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
)

// remoteRetryInterval is how long a remote that failed to connect is left
// before the next attempt: connecting can take connectTimeout, and the
// stacks poll would otherwise pay it every time.
const remoteRetryInterval = 30 * time.Second

// remoteCommands is a command for each remote a stack is pinned to, other
// than the session's own, connected the first time it's asked for.
type remoteCommands struct {
	mutex    sync.Mutex
	commands map[string]*commands.IncusCommand
	failures map[string]remoteFailure

	// connect and known are the CLI config's; tests stand in a second
	// daemon here.
	connect func(name string) (*commands.IncusCommand, error)
	known   func(name string) bool
}

type remoteFailure struct {
	err error
	at  time.Time
}

// commandFor is the command a stack pinned to remote goes through: the
// session's own for no remote, or for the session's remote by name. Off the
// main loop: the first call for a remote connects to it.
func (gui *Gui) commandFor(remote string) (*commands.IncusCommand, error) {
	if remote == "" || remote == gui.IncusCommand.RemoteName {
		return gui.IncusCommand, nil
	}

	return gui.remotes.get(remote)
}

// publishHostFor is where the ports of an instance on remote are reached,
// empty while that remote isn't connected.
func (gui *Gui) publishHostFor(remote string) string {
	if remote == "" || remote == gui.IncusCommand.RemoteName {
		return gui.IncusCommand.PublishHost
	}

	gui.remotes.mutex.Lock()
	defer gui.remotes.mutex.Unlock()

	if command, ok := gui.remotes.commands[remote]; ok {
		return command.PublishHost
	}

	return ""
}

func (r *remoteCommands) get(remote string) (*commands.IncusCommand, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if command, ok := r.commands[remote]; ok {
		return command, nil
	}

	if failure, ok := r.failures[remote]; ok && time.Since(failure.at) < remoteRetryInterval {
		return nil, failure.err
	}

	// Held while connecting, so two fetches don't both dial the remote.
	command, err := r.connect(remote)
	if err != nil {
		if r.failures == nil {
			r.failures = map[string]remoteFailure{}
		}

		r.failures[remote] = remoteFailure{err: err, at: time.Now()}

		return nil, err
	}

	if r.commands == nil {
		r.commands = map[string]*commands.IncusCommand{}
	}

	delete(r.failures, remote)
	r.commands[remote] = command

	return command, nil
}
