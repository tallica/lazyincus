package tui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// failureWatch is pkg/gui's (docs/Panels.md, "Services"): note runs on a listener's goroutine.
type failureWatch struct {
	mutex    sync.Mutex
	watching bool
	project  string
	kept     []daemonFailure
	heard    chan struct{}
}

type daemonFailure struct {
	what string
	err  string
}

// failureGrace: the daemon's event and its answer to incus-compose come down different connections.
const failureGrace = 200 * time.Millisecond

// failureWaitLimit: something failing over and over in the project would never let the wait end.
const failureWaitLimit = time.Second

func (f *failureWatch) watch(project string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.watching, f.project, f.kept, f.heard = true, project, nil, make(chan struct{}, 1)
}

// stop ends the watch, waiting grace for a failure and again after each, up to failureWaitLimit.
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

// note keeps a failure in the watched project once: something retrying a start would repeat it.
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

// streamWait is how long a verb waits for a stream of its own before it runs anyway.
const streamWait = time.Second

// followingVerbs run until the user stops them, so a failure meanwhile is some other client's.
var followingVerbs = []string{"logs", "exec", "console", "monitor"}

// cliVerb is the first of args that is neither a flag nor --project's value.
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

// eventsCover is pkg/gui's: the session's open stream hears project.
func (m *Model) eventsCover(project string) bool {
	scope := m.incus.ProjectName()

	return m.eventsLive && (scope == "" || scope == project)
}

// watchReadyMsg is a failures stream open, or given up on: start runs the verb.
type watchReadyMsg struct {
	start func() tea.Cmd
}

// runWatched is pkg/gui's: then gets what failed in project on remote while cmd ran, if cmd failed too.
func (m *Model) runWatched(cmd *exec.Cmd, remote, project string, then func(failures []daemonFailure) tea.Cmd) tea.Cmd {
	if len(cmd.Args) > 0 && slices.Contains(followingVerbs, cliVerb(cmd.Args[1:])) {
		return m.runOnTerminal(cmd, nil, func(error) tea.Cmd { return then(nil) })
	}

	command, covered := m.incus, false
	if !m.onSessionRemote(remote) {
		// A remote not connected yet goes unwatched rather than hold the verb up.
		command, _ = m.commandFor(remote)
	} else {
		covered = m.eventsCover(project)
	}

	watch := m.failures
	watch.watch(project)

	var failures []daemonFailure

	run := func(stopListening func()) tea.Cmd {
		after := func(runErr error) {
			var grace time.Duration
			if runErr != nil {
				grace = failureGrace
			}

			failures = watch.stop(grace)
			stopListening()
		}

		return m.runOnTerminal(cmd, after, func(runErr error) tea.Cmd {
			// The terminal may not have been released, and after not run.
			watch.stop(0)
			stopListening()

			if runErr == nil {
				return then(nil)
			}

			// With no prompt to return, nothing else says the command failed.
			if len(failures) == 0 && m.cfg.UserConfig.Gui.ReturnImmediately {
				failures = []daemonFailure{{what: strings.Join(cmd.Args, " "), err: fmt.Sprintf(m.tr.FailedOnTerminal, runErr)}}
			}

			return then(failures)
		})
	}

	switch {
	case covered:
		return run(func() {})
	case command == nil:
		watch.stop(0)
		return run(func() {})
	}

	return func() tea.Msg {
		stopListening, ok := listenForFailures(command, project, watch)
		if !ok {
			watch.stop(0)
			stopListening = func() {}
		}

		return watchReadyMsg{start: func() tea.Cmd { return run(stopListening) }}
	}
}

func listenForFailures(command *commands.IncusCommand, project string, watch *failureWatch) (func(), bool) {
	ctx, cancel := context.WithCancel(context.Background())
	opened, done := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(done)

		err := command.ListenForEventsIn(ctx, project, []string{api.EventTypeOperation}, func() { close(opened) }, watch.note)
		if err != nil {
			command.Log.Warn().Err(err).Msg("failures stream")
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

// showFailures is pkg/gui's: one Error popup for every failure, if any.
func (m *Model) showFailures(failures []daemonFailure, remote string) {
	if len(failures) == 0 {
		return
	}

	messages := make([]string, len(failures))
	for i, failure := range failures {
		messages[i] = m.onRemote(failure.what, remote) + ": " + failure.err
	}

	m.openError(strings.Join(messages, "\n\n"))
}
