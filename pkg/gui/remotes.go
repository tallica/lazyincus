package gui

import (
	"os"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/types"
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

	// connect, known and names are the CLI config's; tests stand in a
	// second daemon here.
	connect func(name string) (*commands.IncusCommand, error)
	known   func(name string) bool
	names   func() []string
}

type remoteFailure struct {
	err error
	at  time.Time
}

// commandFor is the command a stack pinned to remote goes through: the
// session's own for no remote, or for the session's remote by name. Off the
// main loop: the first call for a remote connects to it.
func (gui *Gui) commandFor(remote string) (*commands.IncusCommand, error) {
	if remote == "" || remote == gui.IncusCommand.RemoteName() {
		return gui.IncusCommand, nil
	}

	return gui.remotes.get(remote)
}

// publishHostFor is where the ports of an instance on remote are reached,
// empty while that remote isn't connected.
func (gui *Gui) publishHostFor(remote string) string {
	if remote == "" || remote == gui.IncusCommand.RemoteName() {
		return gui.IncusCommand.PublishHost()
	}

	gui.remotes.mutex.Lock()
	defer gui.remotes.mutex.Unlock()

	if command, ok := gui.remotes.commands[remote]; ok {
		return command.PublishHost()
	}

	return ""
}

func (r *remoteCommands) get(remote string) (*commands.IncusCommand, error) {
	return r.connected(remote, false)
}

// connected is remote's command, connecting if there's none; retry tries
// a remote that failed recently anyway, for someone asking for it by name.
func (r *remoteCommands) connected(remote string, retry bool) (*commands.IncusCommand, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if command, ok := r.commands[remote]; ok {
		return command, nil
	}

	if failure, ok := r.failures[remote]; ok && !retry && time.Since(failure.at) < remoteRetryInterval {
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

// handleSwitchRemote is `R`: the CLI's remotes that hold instances, to move
// every panel onto one for the rest of the session. `incus remote switch` is
// the lasting way.
func (gui *Gui) handleSwitchRemote(g *gocui.Gui, v *gocui.View) error {
	current := gui.IncusCommand.RemoteName()

	items := lo.Map(gui.remotes.names(), func(name string, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: []string{marker(name == current), name},
			OnPress: func() error {
				return gui.switchToRemote(name)
			},
		}
	})

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.RemotesTitle,
		Items: items,
	})
}

// switchToRemote connects off the main loop, and only once that has worked
// lets go of the remote the panels are on: a remote that doesn't answer
// leaves everything where it was.
func (gui *Gui) switchToRemote(name string) error {
	if name == gui.IncusCommand.RemoteName() {
		return nil
	}

	return gui.WithWaitingStatus(gui.Tr.ConnectingStatus, func() error {
		command, err := gui.remotes.connected(name, true)
		if err != nil {
			return err
		}

		gui.g.Update(func(*gocui.Gui) error {
			gui.IncusCommand.UseRemote(command)

			// The shell-outs read it, as they did --remote's.
			if err := os.Setenv("INCUS_REMOTE", name); err != nil {
				return err
			}

			return gui.reloadAfterScopeChange()
		})

		return nil
	})
}
