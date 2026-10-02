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

// Resting on a stack moves the rest of the screen to its remote; the
// first stack, selected at startup, leaves --remote's.
func TestTheScreenFollowsTheSelectedStack(t *testing.T) {
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
		s.gui.remoteFollow.delay = 0
	})
	order := func() []string {
		return onLoop(t, s, func() []string {
			return lo.Map(s.gui.Panels.Stacks.List.GetItems(), func(stack *commands.ComposeStack, _ int) string {
				return stack.Name
			})
		})
	}

	// zoo was saved with no remote, so it sorts first.
	s.settle(t, "Services (zoo)")
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())
	assert.Equal(t, "fake", s.gui.IncusCommand.RemoteName())

	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "Services (default)")
	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "(pve01/all projects)")
	require.Eventually(t, func() bool {
		return slices.Equal(instanceNames(t, s), []string{"pve01:api-1"})
	}, 5*time.Second, 20*time.Millisecond)
	s.settle(t, "Services (shop)")
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())

	s.do(t, s.gui.Panels.Stacks.HandlePrevLine)
	s.settle(t, "(fake/all projects)")
	s.settle(t, "Services (default)")
}

// An R stands until the selection moves: a refresh reselecting the same
// stack doesn't take the screen back to it.
func TestARemoteSwitchStandsUntilTheSelectionMoves(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, here)(s)
		withRemotes(map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})})(s)
		s.gui.remoteFollow.delay = 0
	})
	s.settle(t, "Services (default)")

	s.do(t, func() error { return s.gui.switchToRemote("pve01") })
	s.settle(t, "Services (default on fake)")

	require.NoError(t, s.gui.refresh(nil, s.gui.fetchStacks))
	s.do(t, s.gui.Panels.Stacks.RerenderList)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "pve01", onLoop(t, s, s.gui.IncusCommand.RemoteName))
}

// A stack whose remote doesn't answer leaves the screen where it was.
func TestFollowingAnUnreachableStack(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"
	down := testStack(t, t.TempDir(), "shop", "api")
	down.Remote = "down"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, here, down)(s)
		withRemotes(map[string]*incustest.Server{})(s)
		s.gui.remoteFollow.delay = 0
	})
	// down sorts first, so startup selects it, and keeps --remote's.
	s.settle(t, "Services (shop on down)")

	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "Services (default)")
	s.do(t, s.gui.Panels.Stacks.HandlePrevLine)
	s.settle(t, "Services (shop on down)")
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "fake", onLoop(t, s, s.gui.IncusCommand.RemoteName))
}
