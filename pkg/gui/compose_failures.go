package gui

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// composeFailures collects the operations that fail in a stack's project
// while a compose verb has the terminal (docs/Panels.md, "Services").
type composeFailures struct {
	mutex    sync.Mutex
	watching bool
	project  string
	kept     []composeFailure
	// failed closes with the first failure the watch keeps.
	failed chan struct{}
}

// composeFailure is a failed operation: what it was doing, which the popup
// names the remote after, and the daemon's error.
type composeFailure struct {
	what string
	err  string
}

// composeFailureGrace is how long a failed verb's failure may trail its
// exit: the daemon's event and its answer to incus-compose come down
// different connections.
const composeFailureGrace = 200 * time.Millisecond

func (f *composeFailures) watch(project string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.watching, f.project, f.kept, f.failed = true, project, nil, make(chan struct{})
}

// stop ends the watch and returns what failed during it, waiting up to
// grace for a first failure if none has arrived.
func (f *composeFailures) stop(grace time.Duration) []composeFailure {
	f.mutex.Lock()
	watching, failed := f.watching, f.failed
	f.mutex.Unlock()

	if watching && grace > 0 {
		select {
		case <-failed:
		case <-time.After(grace):
		}
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	kept := f.kept
	f.watching, f.project, f.kept, f.failed = false, "", nil, nil

	return kept
}

// note takes in an operation event, keeping it if it's a failure in the
// project being watched that it hasn't kept already: something retrying a
// start would otherwise fill the popup with the same line.
func (f *composeFailures) note(event commands.Event) {
	if event.Status != api.Failure || event.Details == nil || event.Details.Operation.Err == "" {
		return
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.watching || event.Project != f.project {
		return
	}

	failure := composeFailure{
		what: strings.TrimSpace(event.Action + " " + strings.Join(event.Instances, ", ")),
		err:  event.Details.Operation.Err,
	}

	if slices.Contains(f.kept, failure) {
		return
	}

	f.kept = append(f.kept, failure)

	if len(f.kept) == 1 {
		close(f.failed)
	}
}

// remoteStreamWait is how long a verb on another remote waits for that
// remote's event stream to open before it runs anyway.
const remoteStreamWait = time.Second

// listenForFailures opens an event stream on remote for composeFailures,
// the session's own being its remote's alone, and returns what closes it.
// A remote not connected yet goes unwatched rather than hold the verb up.
func (gui *Gui) listenForFailures(remote string) (func(), bool) {
	command, err := gui.remotes.get(remote)
	if err != nil {
		return nil, false
	}

	ctx, cancel := context.WithCancel(context.Background())
	opened, done := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(done)

		err := command.ListenForEvents(ctx, []string{api.EventTypeOperation}, func() { close(opened) }, gui.composeFailures.note)
		if err != nil {
			gui.Log.Warn().Err(err).Str("remote", remote).Msg("compose failures stream")
		}
	}()

	select {
	case <-opened:
	case <-done:
		cancel()
		return nil, false
	case <-time.After(remoteStreamWait):
	}

	return cancel, true
}

// runComposeWatched runs cmd, an incus-compose command on target's stack, on
// the terminal, and returns the operations that failed in its project if it
// failed too.
func (gui *Gui) runComposeWatched(cmd *exec.Cmd, target composeTarget) ([]composeFailure, error) {
	if gui.onSessionRemote(target.remote) {
		gui.composeFailures.watch(target.project)
	} else if stopListening, ok := gui.listenForFailures(target.remote); ok {
		defer stopListening()
		gui.composeFailures.watch(target.project)
	}

	runErr, err := gui.runCompose(cmd)

	var grace time.Duration
	if runErr != nil {
		grace = composeFailureGrace
	}

	failures := gui.composeFailures.stop(grace)
	if err != nil || runErr == nil {
		return nil, err
	}

	return failures, nil
}

// showComposeFailures puts failures from a stack on remote in an Error
// popup, if there are any.
func (gui *Gui) showComposeFailures(failures []composeFailure, remote string) error {
	if len(failures) == 0 {
		return nil
	}

	messages := make([]string, len(failures))
	for i, failure := range failures {
		messages[i] = gui.onRemote(failure.what, remote) + ": " + failure.err
	}

	return gui.createErrorPanel(strings.Join(messages, "\n\n"))
}
