package gui

import (
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
)

// withImageUpdates runs a stack of web, db and cache, each on an image of
// its own, and has the registries answer with states, by service.
func withImageUpdates(t *testing.T, states map[string]commands.UpstreamState) func(*screen) {
	t.Helper()

	return func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web", "db", "cache"))(s)

		instances := make([]api.InstanceFull, 0, 3)
		for _, service := range []string{"web", "db", "cache"} {
			instance := composeFixture("default", service+"-1", service)
			instance.Config = map[string]string{"volatile.base_image": service + "-image"}
			instances = append(instances, instance)
		}
		s.server.SetInstances(instances)

		s.gui.checkUpstreams = func(context.Context, []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
			results := map[string]commands.UpstreamStatus{}
			for service, state := range states {
				results[service+"-image"] = commands.UpstreamStatus{
					State: state, Ref: "docker.io/library/" + service + ":1",
					From: &commands.BuildInfo{Version: "1.0"}, To: &commands.BuildInfo{Version: "1.1"},
				}
			}

			return results
		}
	}
}

// G lists the stack's services with a newer image build, every one ticked,
// and recreates on a pull the ones left ticked - and only those.
func TestUpdateImagesRecreatesTheTicked(t *testing.T) {
	var (
		ran   []string
		mutex sync.Mutex
	)

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withImageUpdates(t, map[string]commands.UpstreamState{
			"web": commands.UpstreamNewer, "db": commands.UpstreamNewer, "cache": commands.UpstreamCurrent,
		})(s)
		s.gui.runOnTerminal = func(cmd *exec.Cmd, after func(error)) (error, error) {
			mutex.Lock()
			ran = cmd.Args[1:]
			mutex.Unlock()

			if after != nil {
				after(nil)
			}

			return nil, nil
		}
	})
	landed(t, s, "stacks")
	require.Eventually(t, func() bool { return len(serviceNames(t, s)) == 3 }, 5*time.Second, 20*time.Millisecond)

	s.press(t, 'G')
	menu := s.settle(t, "Update images")
	assert.Regexp(t, `\[x\] db +docker\.io/library/db:1 +1\.0 → 1\.1`, rowWith(menu, "[x] db"))
	assert.Contains(t, menu, "[x] web")
	assert.NotContains(t, menu, "cache:1")

	s.pressKey(t, tcell.Key(gocui.KeySpace))
	s.settle(t, "[ ] db")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "pull the newer images of web and recreate")
	s.press(t, 'y')

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()

		return ran != nil
	}, 5*time.Second, 20*time.Millisecond)

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, []string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach", "web"}, ran)
}

// Checks of other images, a remote's that answered at the last check, say
// nothing of this stack's: G checks again rather than find nothing newer.
func TestUpdateImagesChecksAStackNotYetChecked(t *testing.T) {
	var asked atomic.Int32

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withImageUpdates(t, map[string]commands.UpstreamState{"web": commands.UpstreamNewer})(s)

		check := s.gui.checkUpstreams
		s.gui.checkUpstreams = func(ctx context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
			asked.Add(1)
			return check(ctx, targets)
		}

		elsewhere := map[string]commands.UpstreamStatus{"elsewhere-image": {State: commands.UpstreamCurrent}}
		s.gui.imageUpdates.Store(&elsewhere)
	})
	landed(t, s, "stacks")
	require.Eventually(t, func() bool { return len(serviceNames(t, s)) == 3 }, 5*time.Second, 20*time.Millisecond)

	s.press(t, 'G')
	s.settle(t, "[x] web")
	assert.Equal(t, int32(1), asked.Load())

	s.pressKey(t, tcell.KeyEsc)
	s.press(t, 'G')
	s.settle(t, "[x] web")
	assert.Equal(t, int32(1), asked.Load(), "checked already")
}

func TestUpdateImagesWithNothingNewer(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, withImageUpdates(t, map[string]commands.UpstreamState{
		"web": commands.UpstreamCurrent, "db": commands.UpstreamFailed,
	}))
	landed(t, s, "services")
	require.Eventually(t, func() bool { return len(serviceNames(t, s)) == 3 }, 5*time.Second, 20*time.Millisecond)

	s.press(t, 'G')
	s.settle(t, "no service of default has a newer image build")
}

func TestUpdateImagesRefusedReadOnly(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withImageUpdates(t, map[string]commands.UpstreamState{"web": commands.UpstreamNewer})(s)
		s.gui.Config.ReadOnly = true
	})
	landed(t, s, "stacks")
	require.Eventually(t, func() bool { return len(serviceNames(t, s)) == 3 }, 5*time.Second, 20*time.Millisecond)

	s.press(t, 'G')
	s.settle(t, "started with --read-only")
}
