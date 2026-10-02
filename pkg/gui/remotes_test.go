package gui

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func instanceNames(t *testing.T, s *screen) []string {
	t.Helper()

	return onLoop(t, s, func() []string {
		return lo.Map(s.gui.Panels.Instances.List.GetAllItems(), func(instance *commands.Instance, _ int) string {
			return instance.Remote + ":" + instance.Name
		})
	})
}

// R lists the remotes with instances, the session's marked.
func TestTheRemotesMenu(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, withRemotes(map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})}))
	s.ready(t)

	s.press(t, 'R')
	screen := s.settle(t, s.gui.Tr.RemotesTitle)
	assert.Regexp(t, `│\*\s+fake`, screen)
	assert.Regexp(t, `│\s+pve01`, screen)
}

// Switching moves every panel, the footer and the shell-outs onto the new
// remote, and a stack pinned to the one left keeps reading from it.
func TestSwitchingRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	pve01 := incustest.New(incustest.Server{
		Version:   "7.5",
		Instances: []api.InstanceFull{composeFixture("default", "only-on-pve01", "")},
	})
	pinned := testStack(t, t.TempDir(), "default", "web")
	pinned.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		labelled(s, map[string]string{"web": "web"})
	})
	s.settle(t, "Services (default)")

	s.do(t, func() error { return s.gui.switchToRemote("pve01") })

	s.settle(t, "Incus v7.5 (pve01/all projects)")
	require.Eventually(t, func() bool {
		return slices.Equal(instanceNames(t, s), []string{"pve01:only-on-pve01"})
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "pve01", os.Getenv("INCUS_REMOTE"))

	// fake is a remote like any other now, and still answers for its stack.
	s.settle(t, "Services (default on fake)")
	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"default/web"})
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "Running", onLoop(t, s, func() string {
		return s.gui.Panels.Stacks.List.GetAllItems()[0].Status()
	}))
}

// A remote that doesn't answer leaves everything where it was.
func TestSwitchingToAnUnreachableRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	s := startScreenWith(t, 140, 40, nil, withRemotes(map[string]*incustest.Server{"pve01": nil}))
	before := s.ready(t)
	names := instanceNames(t, s)

	s.do(t, func() error { return s.gui.switchToRemote("down") })

	s.settle(t, "no answer")
	assert.Contains(t, before, "(fake/all projects)")
	assert.Equal(t, "fake", s.gui.IncusCommand.RemoteName())
	assert.Equal(t, "fake", os.Getenv("INCUS_REMOTE"))
	assert.Equal(t, names, instanceNames(t, s))
}

// Moving through the stacks moves nothing else; space on one moves the
// rest of the screen to its remote, and the list stays as it was.
func TestSpaceSwitchesToTheStacksRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	pve01 := incustest.New(incustest.Server{
		Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")},
	})
	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"
	there := testStack(t, t.TempDir(), "shop", "api")
	there.Remote = "pve01"
	// Saved before remotes: it follows the session, but keeps its place.
	plain := testStack(t, t.TempDir(), "zoo", "keeper")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, here, there, plain)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
	})

	order := func() []string {
		return onLoop(t, s, func() []string {
			return lo.Map(s.gui.Panels.Stacks.List.GetItems(), func(stack *commands.ComposeStack, _ int) string {
				return stack.Name
			})
		})
	}
	selected := func() *commands.ComposeStack {
		return onLoop(t, s, func() *commands.ComposeStack {
			stack, _ := s.gui.Panels.Stacks.GetSelectedItem()
			return stack
		})
	}

	// zoo was saved with no remote, so it sorts first.
	s.settle(t, "Services (zoo)")
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())

	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "Services (shop on pve01)")
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "fake", onLoop(t, s, s.gui.IncusCommand.RemoteName))

	stack := selected()
	s.do(t, func() error { return s.gui.stackSwitchRemote(stack) })
	s.settle(t, "(pve01/all projects)")
	require.Eventually(t, func() bool {
		return slices.Equal(instanceNames(t, s), []string{"pve01:api-1"})
	}, 5*time.Second, 20*time.Millisecond)
	s.settle(t, "Services (shop)")
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())

	// The session's remote is the one marked.
	screen := s.snapshot(t)
	assert.Regexp(t, `│\* pve01 +shop`, screen)
	assert.Regexp(t, `│  fake +default`, screen)

	// Space on a stack already on the session's remote moves nothing.
	s.do(t, s.gui.Panels.Stacks.HandlePrevLine)
	s.settle(t, "Services (default on fake)")
	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	stack = selected()
	s.do(t, func() error { return s.gui.stackSwitchRemote(stack) })
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "pve01", onLoop(t, s, s.gui.IncusCommand.RemoteName))
}

// Space on a stack whose remote doesn't answer leaves the screen where it
// was.
func TestSwitchingToAnUnreachableStack(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	down := testStack(t, t.TempDir(), "shop", "api")
	down.Remote = "down"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, down)(s)
		withRemotes(map[string]*incustest.Server{})(s)
	})
	s.settle(t, "Services (shop on down)")

	s.do(t, func() error {
		stack, err := s.gui.Panels.Stacks.GetSelectedItem()
		if err != nil {
			return err
		}

		return s.gui.stackSwitchRemote(stack)
	})
	s.settle(t, "no answer")
	assert.Equal(t, "fake", onLoop(t, s, s.gui.IncusCommand.RemoteName))
}

// A remote slow to connect holds up nothing: its stacks read connecting,
// the others and the instances carry on, and it fills in once it answers.
func TestASlowRemoteHoldsUpNothing(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"
	slow := testStack(t, t.TempDir(), "shop", "api")
	slow.Remote = "pve01"

	answer := make(chan struct{})
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, here, slow)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		labelled(s, map[string]string{"web": "web"})

		connect := s.gui.remotes.connect
		s.gui.remotes.connect = func(remote string) (*commands.IncusCommand, error) {
			if remote == "pve01" {
				<-answer
			}

			return connect(remote)
		}
	})
	t.Cleanup(func() {
		select {
		case <-answer:
		default:
			close(answer)
		}
	})

	statusOf := func(name string) string {
		return onLoop(t, s, func() string {
			for _, stack := range s.gui.Panels.Stacks.List.GetAllItems() {
				if stack.Name == name {
					if stack.StatusPending {
						return "connecting"
					}

					return stack.Status()
				}
			}

			return ""
		})
	}

	require.Eventually(t, func() bool {
		return statusOf("default") == "Running" && statusOf("shop") == "connecting"
	}, 5*time.Second, 20*time.Millisecond)
	assert.NotEmpty(t, instanceNames(t, s))

	close(answer)
	require.Eventually(t, func() bool { return statusOf("shop") == "Running" }, 5*time.Second, 20*time.Millisecond)
}
