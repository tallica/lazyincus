package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func TestComposeFailuresIgnoreTokenOperations(t *testing.T) {
	var failures failureWatch
	failures.watch("dns")

	token := failedOperation("dns", "console refused")
	token.Details.Operation.Class = "token"
	failures.note(token)

	assert.Empty(t, failures.stop(0))
}

// Something else failing over and over in the project can't hold the screen.
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

func TestFailureWatchOnEveryProject(t *testing.T) {
	var failures failureWatch
	failures.watch("")

	failures.note(failedOperation("dns", "one"))
	failures.note(failedOperation("shop", "other"))

	assert.Len(t, failures.stop(0), 2)
}

func TestCLIVerb(t *testing.T) {
	for _, args := range [][]string{
		{"--project", "dns", "exec", "web", "--", "sh"},
		{"--project=dns", "exec", "web"},
		{"-q", "exec", "--project", "dns", "web"},
	} {
		assert.Equal(t, "exec", cliVerb(args), args)
	}

	assert.Equal(t, "logs", cliVerb([]string{"logs", "--follow"}))
}

func TestCLIProject(t *testing.T) {
	for _, args := range [][]string{
		{"--project", "dns", "exec", "web", "--", "sh"},
		{"--project=dns", "exec", "web"},
		{"-q", "exec", "--project", "dns", "web"},
	} {
		assert.Equal(t, "dns", cliProject(args), args)
	}

	assert.Empty(t, cliProject([]string{"start", "web"}))
}

// A start from either panel that fails on the daemon says why once the
// terminal hands the screen back, the failure arriving before the verb exits
// or just after, and from a stack on another remote too.
func TestAFailedStartShowsTheDaemonsError(t *testing.T) {
	for _, how := range []string{"key", ":"} {
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
		t.Run(view, func(t *testing.T) { failedStart(t, failedStartCase{view: view, project: "shop", sessionStream: true}) })
	}
}

// The session's open stream carries the stack's project: the verb opens none of its own.
func TestAFailedStartIsHeardOnTheSessionsStream(t *testing.T) {
	for name, late := range map[string]bool{"during": false, "after exit": true} {
		t.Run(name, func(t *testing.T) { failedStart(t, failedStartCase{view: "stacks", late: late, sessionStream: true}) })
	}
}

type failedStartCase struct {
	// remote is the stack's, "" for the session's.
	remote string
	view   string
	// late has the failure arrive after the verb exits.
	late bool
	// how starts it: "key", or typed at ":"; "" is "key".
	how string
	// project narrows the session to it, if it's not "".
	project string
	// sessionStream opens the session's event stream; without it the verb opens its own.
	sessionStream bool
}

func failedStart(t *testing.T, c failedStartCase) {
	t.Helper()

	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = c.remote
	session := incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{composeFixture("default", "web", "web")}})
	pve01 := incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{composeFixture("default", "web", "web")}})

	opts := options{compose: true, server: session, remotes: map[string]*incustest.Server{"pve01": pve01}}
	daemon := session
	if c.remote == "" {
		opts.localStack = stack
	} else {
		opts.savedStacks = []*commands.ComposeStack{stack}
		daemon = pve01
	}

	m := start(t, opts, 140, 40)
	if c.project != "" {
		m.incus.UseProject(c.project)
	}
	if c.sessionStream {
		listening(t, m)
		require.Equal(t, c.project == "", m.eventsCover("default"))
	}
	if c.remote != "" {
		press(m, "1")
		untilRemotes(t, m, func() bool { return len(m.serviceRows) == 1 })
	}

	failure := operationEvent(t, api.Operation{
		ID: "start", Class: "task", Description: "Starting instance",
		Status: "Failure", StatusCode: api.Failure, Err: "port 53 is taken",
		Resources: map[string][]string{"instances": {"/1.0/instances/web"}},
	})

	m.runOnTerminal = func(_ *exec.Cmd, after func(error), then func(error) tea.Cmd) tea.Cmd {
		listening := 1
		if c.sessionStream && c.project != "" {
			listening = 2
		}
		require.Eventually(t, func() bool { return daemon.Listening() >= listening }, 5*time.Second, 10*time.Millisecond)

		if c.late {
			time.AfterFunc(20*time.Millisecond, func() { daemon.Emit(failure) })
		} else {
			daemon.Emit(failure)
			require.Eventually(t, func() bool {
				m.failures.mutex.Lock()
				defer m.failures.mutex.Unlock()

				return len(m.failures.kept) == 1
			}, 5*time.Second, 10*time.Millisecond)
		}

		runErr := errors.New("exit status 1")
		after(runErr)

		return then(runErr)
	}

	press(m, map[string]string{"stacks": "1", "services": "2"}[c.view])
	require.Equal(t, c.view, m.currentView())
	switch c.how {
	case ":":
		press(m, ":")
		typeText(m, "start web")
		press(m, "enter")
	default:
		press(m, "S")
	}

	want := "Starting instance web: port 53 is taken"
	if c.remote != "" {
		want = "Starting instance web on " + c.remote + ": port 53 is taken"
	}
	assert.Contains(t, popupText(m), want)
}

// A command that fails with nothing failing on the daemon says so in a popup
// under gui.returnImmediately, where nothing else would; with the prompt to
// return, the terminal already has.
func TestAFailureOffTheDaemonPopsUpOnlyWhenReturningImmediately(t *testing.T) {
	for _, immediately := range []bool{true, false} {
		t.Run(map[bool]string{true: "immediately", false: "prompted"}[immediately], func(t *testing.T) {
			m, _, _ := composeModel(t, options{configure: func(c *config.UserConfig) { c.Gui.ReturnImmediately = immediately }})
			m.runOnTerminal = func(_ *exec.Cmd, after func(error), then func(error) tea.Cmd) tea.Cmd {
				runErr := errors.New("exit status 1")
				after(runErr)

				return then(runErr)
			}

			press(m, "S")

			if immediately {
				assert.Contains(t, popupText(m), "incus-compose start: exit status 1 - what it printed went by")
			} else {
				assert.NotContains(t, popupText(m), "what it printed went by")
			}
		})
	}
}

// A verb that went through shows nothing, whatever failed meanwhile is someone else's.
func TestASucceededVerbShowsNoFailure(t *testing.T) {
	m, ran, _ := composeModel(t, options{})
	press(m, "S")

	require.Len(t, *ran, 1)
	assert.NotContains(t, popupText(m), "Error")
}
