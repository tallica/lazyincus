package gui

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/types"
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
	byProject map[string]map[string][]string
	err       error
}

// commandFor is the command a stack pinned to remote goes through: the
// session's own for no remote, or for the session's remote by name. The
// first call for another remote starts connecting to it, and answers
// errConnecting until that's done.
func (gui *Gui) commandFor(remote string) (*commands.IncusCommand, error) {
	if remote == "" || remote == gui.IncusCommand.RemoteName() {
		return gui.IncusCommand, nil
	}

	if !gui.remotes.known(remote) {
		return nil, gui.unknownRemote(remote)
	}

	return gui.remotes.get(remote)
}

// unknownRemote is a stack's remote gone from the CLI's config, renamed or
// removed since the stack was saved, saying how to point the stack at
// another.
func (gui *Gui) unknownRemote(remote string) error {
	return fmt.Errorf(gui.Tr.UnknownRemote, remote)
}

// refreshForRemote re-reads the stacks and services once another remote
// has something new to say: a connection made, or statuses that changed.
func (gui *Gui) refreshForRemote() {
	if gui.isStopped() {
		return
	}

	if err := gui.refresh(nil, gui.fetchStacks, gui.fetchServices); err != nil {
		gui.Log.Warn(err)
	}
}

// remoteStatusErr is why another remote's statuses couldn't be read last
// time, without asking it: a remote known to be away is no reason to wait
// out a request to it.
func (r *remoteCommands) remoteStatusErr(remote string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.statuses[remote].err
}

// onRemote is what followed by the remote it's on, when that isn't the
// session's: confirmations and titles, where the other panels would
// otherwise vouch for the wrong daemon.
func (gui *Gui) onRemote(what, remote string) string {
	if gui.onSessionRemote(remote) {
		return what
	}

	return fmt.Sprintf(gui.Tr.OnRemote, what, remote)
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

// connectNow is get waiting for the connection, and trying a remote that
// failed recently anyway: for someone who asked for that remote by name.
func (r *remoteCommands) connectNow(remote string) (*commands.IncusCommand, error) {
	r.mutex.Lock()
	command, ok := r.commands[remote]
	r.mutex.Unlock()

	if ok {
		return command, nil
	}

	command, err := r.connect(remote)
	r.store(remote, command, err)

	return command, err
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
func (r *remoteCommands) cachedStatuses(remote string, read func() (map[string]map[string][]string, error), changed func()) (remoteStatuses, bool) {
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
		command, err := gui.remotes.connectNow(name)
		if err != nil {
			return err
		}

		gui.g.Update(func(*gocui.Gui) error {
			return gui.moveToRemote(name, command)
		})

		return nil
	})
}

// moveToRemote puts every panel on command's remote. Main loop only.
func (gui *Gui) moveToRemote(name string, command *commands.IncusCommand) error {
	gui.IncusCommand.UseRemote(command)

	// From the stacks already listed, rather than once every panel has been
	// read again from the new remote; the stacks read then confirms it.
	if !gui.composeUnavailable() {
		gui.State.StacksHere = gui.stacksHere(gui.Panels.Stacks.List.GetAllItems())
		if err := gui.landOffStacks(); err != nil {
			return err
		}
	}

	gui.State.Landing = true

	// The shell-outs read it, as they did --remote's.
	if err := os.Setenv("INCUS_REMOTE", name); err != nil {
		return err
	}

	return gui.reloadAfterScopeChange()
}

// stackSwitchRemote is space on Stacks: the rest of the screen to the
// stack's remote, the way `R` would. One already there has nowhere to go.
func (gui *Gui) stackSwitchRemote(stack *commands.ComposeStack) error {
	if gui.onSessionRemote(stack.Remote) {
		return nil
	}

	if !gui.remotes.known(stack.Remote) {
		return gui.createErrorPanel(gui.unknownRemote(stack.Remote).Error())
	}

	return gui.switchToRemote(stack.Remote)
}
