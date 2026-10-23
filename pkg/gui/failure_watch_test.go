package gui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func failedOperation(project, err string) commands.Event {
	return commands.Event{
		Type: api.EventTypeOperation, Project: project, Action: "Starting instance",
		Status: api.Failure, Instances: []string{"dns-server"},
		Details: &commands.Operation{Operation: api.Operation{StatusCode: api.Failure, Err: err}},
	}
}

func TestComposeFailuresKeepTheWatchedProjectsFailures(t *testing.T) {
	var failures failureWatch

	failures.note(failedOperation("dns", "before the watch"))
	failures.watch("dns")
	failures.note(failedOperation("other", "another project's"))
	failures.note(failedOperation("dns", "Failed to listen on 0.0.0.0:53"))
	failures.note(failedOperation("dns", "Failed to listen on 0.0.0.0:53"))

	succeeded := failedOperation("dns", "")
	succeeded.Status = api.Success
	failures.note(succeeded)

	assert.Equal(t, []daemonFailure{{what: "Starting instance dns-server", err: "Failed to listen on 0.0.0.0:53"}}, failures.stop(0))

	failures.note(failedOperation("dns", "after the watch"))
	assert.Empty(t, failures.stop(0))
}

// A verb can fail several operations: stop waits out the last, not the first.
func TestStopWaitsOutEveryFailureInABatch(t *testing.T) {
	var failures failureWatch
	failures.watch("dns")

	go func() {
		failures.note(failedOperation("dns", "first"))
		time.Sleep(20 * time.Millisecond)
		failures.note(failedOperation("dns", "second"))
	}()

	kept := failures.stop(200 * time.Millisecond)

	require.Len(t, kept, 2)
	assert.Equal(t, "second", kept[1].err)
}

// The Operations tab leaves token operations out, and so does the popup.
func TestComposeFailuresIgnoreTokenOperations(t *testing.T) {
	var failures failureWatch
	failures.watch("dns")

	token := failedOperation("dns", "console refused")
	token.Details.Operation.Class = "token"
	failures.note(token)

	assert.Empty(t, failures.stop(0))
}

// Something else failing over and over in the project can't hold the
// screen: the wait has a limit however many failures arrive.
func TestStopWaitsNoLongerThanItsLimit(t *testing.T) {
	var failures failureWatch
	failures.watch("dns")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for i := 0; ctx.Err() == nil; i++ {
			failures.note(failedOperation("dns", fmt.Sprintf("attempt %d", i)))
			time.Sleep(20 * time.Millisecond)
		}
	}()

	began := time.Now()
	failures.stop(200 * time.Millisecond)

	assert.Less(t, time.Since(began), failureWaitLimit+500*time.Millisecond)
}

func TestFollowingVerbsAreNotWatched(t *testing.T) {
	assert.True(t, followingVerb(exec.Command("incus-compose", "logs", "--follow")))
	assert.False(t, followingVerb(exec.Command("incus-compose", "up", "web")))
}

// A start from either panel, by its key or typed at ;, that fails on the
// daemon says why once the terminal hands the screen back, the failure
// arriving before the verb exits or just after, and from a stack on another
// remote too.
func TestAFailedStartShowsTheDaemonsError(t *testing.T) {
	for _, typed := range []bool{false, true} {
		for _, remote := range []string{"", "pve01"} {
			for _, view := range []string{"stacks", "services"} {
				for name, late := range map[string]bool{"during": false, "after exit": true} {
					how := map[bool]string{false: "key", true: "typed"}[typed]
					t.Run(how+"/"+cmp.Or(remote, "session")+"/"+view+"/"+name, func(t *testing.T) {
						failedStart(t, failedStartCase{remote: remote, view: view, late: late, typed: typed})
					})
				}
			}
		}
	}
}

// A session narrowed with P to another project still hears the stack's.
func TestAFailedStartShowsTheDaemonsErrorUnderAnotherProject(t *testing.T) {
	for _, view := range []string{"stacks", "services"} {
		t.Run(view, func(t *testing.T) { failedStart(t, failedStartCase{view: view, project: "shop"}) })
	}
}

// failedStartCase is where failedStart's verb runs and how it fails.
type failedStartCase struct {
	// remote is the stack's, "" for the session's.
	remote string
	view   string
	// late has the failure arrive after the verb exits.
	late bool
	// typed runs the verb typed at ; rather than by its key.
	typed bool
	// project narrows the session to it, if it's not "".
	project string
}

func failedStart(t *testing.T, c failedStartCase) {
	t.Helper()

	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = c.remote
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("default", "web", "web")}})

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		if c.project != "" {
			s.gui.IncusCommand.UseProject(c.project)
		}

		if c.remote == "" {
			withStacks(t, stack)(s)
		} else {
			// The local stack is the session remote's; one elsewhere is saved.
			withStacks(t, nil, stack)(s)
		}
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		labelled(s, map[string]string{"web": "web"})
	})

	daemon := s.server
	if c.remote != "" {
		daemon = pve01
	}

	failure := operationEvent(t, api.Operation{
		ID: "start", Class: "task", Description: "Starting instance",
		Status: "Failure", StatusCode: api.Failure, Err: "port 53 is taken",
		Resources: map[string][]string{"instances": {"/1.0/instances/web"}},
	})

	s.gui.runOnTerminal = func(_ *exec.Cmd, after func(error)) (error, error) {
		if c.late {
			time.AfterFunc(20*time.Millisecond, func() { daemon.Emit(failure) })
		} else {
			daemon.Emit(failure)
			// The main loop's goroutine, where require can't stop the test.
			assert.Eventually(t, func() bool {
				s.gui.failureWatch.mutex.Lock()
				defer s.gui.failureWatch.mutex.Unlock()

				return len(s.gui.failureWatch.kept) == 1
			}, 5*time.Second, 10*time.Millisecond)
		}

		runErr := errors.New("exit status 1")
		if after != nil {
			after(runErr)
		}

		return runErr, nil
	}

	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)
	landed(t, s, c.view)
	s.settle(t, "Services (default")
	require.Eventually(t, func() bool {
		return len(onLoop(t, s, s.gui.Panels.Services.List.GetAllItems)) > 0
	}, 5*time.Second, 20*time.Millisecond)

	want := "Starting instance web: port 53 is taken"
	if c.remote != "" {
		want = "Starting instance web on " + c.remote + ": port 53 is taken"
	}

	if c.typed {
		s.press(t, ';')
		s.settle(t, "incus-compose: default")

		for _, key := range "start" {
			s.press(t, key)
		}

		s.pressKey(t, tcell.KeyEnter)
	} else {
		s.press(t, 'S')
	}

	s.settle(t, want)
}
