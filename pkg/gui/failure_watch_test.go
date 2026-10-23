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
	"github.com/tallica/lazyincus/pkg/config"
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

// A watch on no project in particular keeps every project's failures.
func TestFailureWatchOnEveryProject(t *testing.T) {
	var failures failureWatch
	failures.watch("")

	failures.note(failedOperation("dns", "one"))
	failures.note(failedOperation("shop", "other"))

	assert.Len(t, failures.stop(0), 2)
}

func TestCLIVerbAndProject(t *testing.T) {
	for _, args := range [][]string{
		{"--project", "dns", "exec", "web", "--", "sh"},
		{"--project=dns", "exec", "web"},
		{"-q", "exec", "--project", "dns", "web"},
	} {
		assert.Equal(t, "exec", cliVerb(args), args)
		assert.Equal(t, "dns", cliProject(args), args)
	}

	assert.Equal(t, "logs", cliVerb([]string{"logs", "--follow"}))
	assert.Empty(t, cliProject([]string{"start", "web"}))
}

// A start from either panel, by its key, typed at ; or typed at :, that
// fails on the daemon says why once the terminal hands the screen back, the
// failure arriving before the verb exits or just after, and from a stack on
// another remote too.
func TestAFailedStartShowsTheDaemonsError(t *testing.T) {
	for _, how := range []string{"key", ";", ":"} {
		for _, remote := range []string{"", "pve01"} {
			for _, view := range []string{"stacks", "services"} {
				for name, late := range map[string]bool{"during": false, "after exit": true} {
					t.Run(how+"/"+cmp.Or(remote, "session")+"/"+view+"/"+name, func(t *testing.T) {
						failedStart(t, failedStartCase{remote: remote, view: view, late: late, how: how})
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

// A session whose stream hasn't opened still hears it: the verb opens its own.
func TestAFailedStartShowsTheDaemonsErrorWhileTheStreamIsDown(t *testing.T) {
	for _, view := range []string{"stacks", "services"} {
		t.Run(view, func(t *testing.T) { failedStart(t, failedStartCase{view: view, streamDown: true}) })
	}
}

// failedStartCase is where failedStart's verb runs and how it fails.
type failedStartCase struct {
	// remote is the stack's, "" for the session's.
	remote string
	view   string
	// late has the failure arrive after the verb exits.
	late bool
	// how starts it: "key", or typed at ";" or ":"; "" is "key".
	how string
	// project narrows the session to it, if it's not "".
	project string
	// streamDown holds the session's event stream unopened throughout.
	streamDown bool
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

		if c.streamDown {
			t.Cleanup(s.server.HoldListen())
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

	if !c.streamDown {
		require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)
	}
	landed(t, s, c.view)
	s.settle(t, "Services (default")
	require.Eventually(t, func() bool {
		return len(onLoop(t, s, s.gui.Panels.Services.List.GetAllItems)) > 0
	}, 5*time.Second, 20*time.Millisecond)

	want := "Starting instance web: port 53 is taken"
	if c.remote != "" {
		want = "Starting instance web on " + c.remote + ": port 53 is taken"
	}

	switch c.how {
	case ";":
		s.press(t, ';')
		s.settle(t, "incus-compose: default")
		s.typeText(t, "start")
		s.pressKey(t, tcell.KeyEnter)
	case ":":
		s.press(t, ':')
		s.settle(t, "--project default")
		s.typeText(t, "start web")
		s.pressKey(t, tcell.KeyEnter)
	default:
		s.press(t, 'S')
	}

	s.settle(t, want)
}

// A command that fails with nothing failing on the daemon - an instance
// that doesn't exist - says so in a popup under gui.returnImmediately, where
// nothing else would; with the prompt to return, the terminal already has.
func TestAFailureOffTheDaemonPopsUpOnlyWhenReturningImmediately(t *testing.T) {
	for _, immediately := range []bool{true, false} {
		t.Run(map[bool]string{true: "immediately", false: "prompted"}[immediately], func(t *testing.T) {
			stack := testStack(t, t.TempDir(), "default", "web")

			s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
				c.Gui.ReturnImmediately = immediately
			}, func(s *screen) {
				withStacks(t, stack)(s)
				labelled(s, map[string]string{"web": "web"})
			})

			ran := make(chan struct{})
			s.gui.runOnTerminal = func(_ *exec.Cmd, after func(error)) (error, error) {
				defer close(ran)

				runErr := errors.New("exit status 1")
				if after != nil {
					after(runErr)
				}

				return runErr, nil
			}

			landed(t, s, "services")
			s.settle(t, "Services (default")
			s.press(t, ':')
			s.settle(t, "--project default")
			s.typeText(t, "start nosuch")
			s.pressKey(t, tcell.KeyEnter)

			select {
			case <-ran:
			case <-time.After(5 * time.Second):
				t.Fatal("the command never ran")
			}

			if immediately {
				shown := s.settle(t, "went by on the terminal")
				assert.Contains(t, shown, "incus --project default start nosuch: exit status 1")
			} else {
				assert.NotContains(t, s.snapshot(t), "went by on the terminal")
			}
		})
	}
}
