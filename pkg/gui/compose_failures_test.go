package gui

import (
	"cmp"
	"errors"
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
	var failures composeFailures

	failures.note(failedOperation("dns", "before the watch"))
	failures.watch("dns")
	failures.note(failedOperation("other", "another project's"))
	failures.note(failedOperation("dns", "Failed to listen on 0.0.0.0:53"))
	failures.note(failedOperation("dns", "Failed to listen on 0.0.0.0:53"))

	succeeded := failedOperation("dns", "")
	succeeded.Status = api.Success
	failures.note(succeeded)

	assert.Equal(t, []composeFailure{{what: "Starting instance dns-server", err: "Failed to listen on 0.0.0.0:53"}}, failures.stop(0))

	failures.note(failedOperation("dns", "after the watch"))
	assert.Empty(t, failures.stop(0))
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
						failedStart(t, remote, view, late, typed)
					})
				}
			}
		}
	}
}

func failedStart(t *testing.T, remote, view string, late, typed bool) {
	t.Helper()

	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = remote
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("default", "web", "web")}})

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		if remote == "" {
			withStacks(t, stack)(s)
		} else {
			// The local stack is the session remote's; one elsewhere is saved.
			withStacks(t, nil, stack)(s)
		}
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		labelled(s, map[string]string{"web": "web"})
	})

	daemon := s.server
	if remote != "" {
		daemon = pve01
	}

	failure := operationEvent(t, api.Operation{
		ID: "start", Class: "task", Description: "Starting instance",
		Status: "Failure", StatusCode: api.Failure, Err: "port 53 is taken",
		Resources: map[string][]string{"instances": {"/1.0/instances/web"}},
	})

	s.gui.runCompose = func(*exec.Cmd) (error, error) {
		if late {
			time.AfterFunc(20*time.Millisecond, func() { daemon.Emit(failure) })
		} else {
			daemon.Emit(failure)
			// The main loop's goroutine, where require can't stop the test.
			assert.Eventually(t, func() bool {
				s.gui.composeFailures.mutex.Lock()
				defer s.gui.composeFailures.mutex.Unlock()

				return len(s.gui.composeFailures.kept) == 1
			}, 5*time.Second, 10*time.Millisecond)
		}

		return errors.New("exit status 1"), nil
	}

	require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)
	landed(t, s, view)
	s.settle(t, "Services (default")
	require.Eventually(t, func() bool {
		return len(onLoop(t, s, s.gui.Panels.Services.List.GetAllItems)) > 0
	}, 5*time.Second, 20*time.Millisecond)

	want := "Starting instance web: port 53 is taken"
	if remote != "" {
		want = "Starting instance web on " + remote + ": port 53 is taken"
	}

	if typed {
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
