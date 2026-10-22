package gui

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
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

	succeeded := failedOperation("dns", "")
	succeeded.Status = api.Success
	failures.note(succeeded)

	assert.Equal(t, []string{"Starting instance dns-server: Failed to listen on 0.0.0.0:53"}, failures.stop(0))

	failures.note(failedOperation("dns", "after the watch"))
	assert.Empty(t, failures.stop(0))
}

// A start from either panel that fails on the daemon says why once the
// terminal hands the screen back, the failure arriving before the verb
// exits or just after.
func TestAFailedStartShowsTheDaemonsError(t *testing.T) {
	for _, view := range []string{"stacks", "services"} {
		for name, late := range map[string]bool{"during": false, "after exit": true} {
			t.Run(view+"/"+name, func(t *testing.T) {
				s := startScreenWith(t, 140, 40, nil, func(s *screen) {
					withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)
					labelled(s, map[string]string{"web": "web"})
				})

				failure := operationEvent(t, api.Operation{
					ID: "start", Class: "task", Description: "Starting instance",
					Status: "Failure", StatusCode: api.Failure, Err: "port 53 is taken",
					Resources: map[string][]string{"instances": {"/1.0/instances/web"}},
				})

				s.gui.runCompose = func(*exec.Cmd) (error, error) {
					if late {
						time.AfterFunc(20*time.Millisecond, func() { s.server.Emit(failure) })
					} else {
						s.server.Emit(failure)
						// The main loop's goroutine, where require can't stop the test.
						assert.Eventually(t, func() bool {
							s.gui.composeFailures.mutex.Lock()
							defer s.gui.composeFailures.mutex.Unlock()

							return len(s.gui.composeFailures.errs) == 1
						}, 5*time.Second, 10*time.Millisecond)
					}

					return errors.New("exit status 1"), nil
				}

				s.settle(t, "Services (default)")
				require.Eventually(t, func() bool { return s.server.Listening() == 1 }, 5*time.Second, 10*time.Millisecond)
				landed(t, s, view)

				s.press(t, 'S')
				s.settle(t, "Starting instance web: port 53 is taken")
			})
		}
	}
}
