package gui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// failureWatch collects the operations that fail in a project while a
// command has the terminal (docs/Panels.md, "Services").
type failureWatch struct {
	mutex    sync.Mutex
	watching bool
	project  string
	kept     []daemonFailure
	// heard signals each failure the watch keeps.
	heard chan struct{}
}

// daemonFailure is a failed operation: what it was doing and the daemon's error.
type daemonFailure struct {
	what string
	err  string
}

// failureGrace is how long a failed verb's failure may trail its
// exit: the daemon's event and its answer to incus-compose come down
// different connections.
const failureGrace = 200 * time.Millisecond

// failureWaitLimit bounds the whole wait, which each failure extends:
// something else failing over and over in the project would never let up.
const failureWaitLimit = time.Second

// watch starts a watch on project, every project for "".
func (f *failureWatch) watch(project string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.watching, f.project, f.kept, f.heard = true, project, nil, make(chan struct{}, 1)
}

// stop ends the watch and returns what failed during it, waiting grace for
// a failure to arrive and again after each one, up to
// failureWaitLimit: a verb can fail several operations at once.
func (f *failureWatch) stop(grace time.Duration) []daemonFailure {
	f.mutex.Lock()
	watching, heard := f.watching, f.heard
	f.mutex.Unlock()

	if watching && grace > 0 {
		limit := time.After(failureWaitLimit)

		for waiting := true; waiting; {
			select {
			case <-heard:
			case <-time.After(grace):
				waiting = false
			case <-limit:
				waiting = false
			}
		}
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	kept := f.kept
	f.watching, f.project, f.kept, f.heard = false, "", nil, nil

	return kept
}

// note takes in an operation event, keeping it if it's a failure in the
// project being watched that it hasn't kept already: something retrying a
// start would otherwise fill the popup with the same line.
func (f *failureWatch) note(event commands.Event) {
	if event.Status != api.Failure || event.Details == nil || event.Details.Operation.Err == "" || event.Details.IsToken() {
		return
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.watching || (f.project != "" && event.Project != f.project) {
		return
	}

	failure := daemonFailure{
		what: strings.TrimSpace(event.Action + " " + strings.Join(event.Instances, ", ")),
		err:  event.Details.Operation.Err,
	}

	if slices.Contains(f.kept, failure) {
		return
	}

	f.kept = append(f.kept, failure)

	select {
	case f.heard <- struct{}{}:
	default:
	}
}

// streamWait is how long a verb waits for a stream of its own to open
// before it runs anyway.
const streamWait = time.Second

// listenForFailures has failureWatch hear project on remote, opening a
// stream there unless the session's open stream already carries the
// project, and returns what closes it. A remote not connected yet goes
// unwatched rather than hold the command up.
func (gui *Gui) listenForFailures(remote, project string) (func(), bool) {
	command := gui.IncusCommand

	if !gui.onSessionRemote(remote) {
		var err error
		if command, err = gui.commandFor(remote); err != nil {
			return nil, false
		}
	} else if gui.eventsCover(project) {
		return func() {}, true
	}

	ctx, cancel := context.WithCancel(context.Background())
	opened, done := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(done)

		err := command.ListenForEventsIn(ctx, project, []string{api.EventTypeOperation}, func() { close(opened) }, gui.failureWatch.note)
		if err != nil {
			gui.Log.Warn().Err(err).Str("remote", remote).Msg("failures stream")
		}
	}()

	// Open before the command runs, or a failure quicker than the dial is missed.
	select {
	case <-opened:
	case <-done:
		cancel()
		return nil, false
	case <-time.After(streamWait):
	}

	return cancel, true
}

// followingVerbs run until the user stops them, so a failure meanwhile is
// some other client's: incus-compose's and incus's alike.
var followingVerbs = []string{"logs", "exec", "console", "monitor"}

// cliVerb is the first of args, an incus or incus-compose command line
// without its binary, that is neither a flag nor --project's value.
func cliVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			return arg
		}
	}

	return ""
}

// cliProject is the project args name with --project, "" for none.
func cliProject(args []string) string {
	project := ""

	for i, arg := range args {
		if arg == "--project" && i+1 < len(args) {
			project = args[i+1]
		} else if value, ok := strings.CutPrefix(arg, "--project="); ok {
			project = value
		}
	}

	return project
}

// runWatched runs cmd on the terminal and returns the operations that
// failed in project on remote meanwhile, every project's for "", if cmd
// failed too - or, under gui.returnImmediately, cmd's own failure when
// none did.
func (gui *Gui) runWatched(cmd *exec.Cmd, remote, project string) ([]daemonFailure, error) {
	if len(cmd.Args) > 0 && slices.Contains(followingVerbs, cliVerb(cmd.Args[1:])) {
		_, err := gui.runOnTerminal(cmd, nil)

		return nil, err
	}

	gui.failureWatch.watch(project)

	stopListening, ok := gui.listenForFailures(remote, project)
	if ok {
		defer stopListening()
	} else {
		gui.failureWatch.stop(0)
	}

	var failures []daemonFailure

	runErr, err := gui.runOnTerminal(cmd, func(runErr error) {
		var grace time.Duration
		if runErr != nil {
			grace = failureGrace
		}

		failures = gui.failureWatch.stop(grace)
	})

	// Suspend may have failed before the callback ran.
	gui.failureWatch.stop(0)

	if err != nil || runErr == nil {
		return nil, err
	}

	// With no prompt to return, nothing else says the command failed.
	if len(failures) == 0 && gui.Config.UserConfig.Gui.ReturnImmediately {
		failures = []daemonFailure{{
			what: strings.Join(cmd.Args, " "),
			err:  fmt.Sprintf(gui.Tr.FailedOnTerminal, runErr),
		}}
	}

	return failures, nil
}

// showFailures puts failures on remote in an Error popup, if there are any.
func (gui *Gui) showFailures(failures []daemonFailure, remote string) error {
	if len(failures) == 0 {
		return nil
	}

	messages := make([]string, len(failures))
	for i, failure := range failures {
		messages[i] = gui.onRemote(failure.what, remote) + ": " + failure.err
	}

	return gui.createErrorPanel(strings.Join(messages, "\n\n"))
}
