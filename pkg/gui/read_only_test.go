package gui

import (
	"slices"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestReadOnlyRemoteRefusesChanges(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	})
	assert.Contains(t, s.ready(t), "read-only")

	s.press(t, 's')
	s.settle(t, "fake is read-only")
}

func TestReadOnlyFlagRefusesChanges(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) { s.gui.Config.ReadOnly = true })
	s.ready(t)

	s.press(t, 's')
	s.settle(t, "started with --read-only")
}

// What a key changes is on the stack's remote, not the session's: a
// read-only remote's stack refuses, and the session's instances don't.
func TestReadOnlyFollowsTheStacksRemote(t *testing.T) {
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"pve01": {ReadOnly: true}}
	}, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
	})

	landed(t, s, "instances")
	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"shop/api"})
	}, 5*time.Second, 20*time.Millisecond)

	screen := s.ready(t)
	assert.NotContains(t, screen, "read-only")

	s.press(t, 's')
	s.settle(t, "Are you sure")
	s.press(t, 'n')

	s.press(t, '1')
	s.settle(t, "Directory:")
	s.press(t, 'S')
	s.settle(t, "pve01 is read-only")
}

// Snapshots can follow a service on another remote, so a snapshot's keys
// answer to its instance's remote.
func TestReadOnlyFollowsTheSnapshotsRemote(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"pve01": {ReadOnly: true}}
	})
	s.ready(t)

	remote := onLoop(t, s, func() string {
		s.gui.Panels.Snapshots.SetItems([]*commands.Snapshot{
			{Owner: "api-1", Name: "snap0", Instance: &commands.Instance{Name: "api-1", Remote: "pve01"}},
		})

		return s.gui.actionRemote("snapshots")
	})
	assert.Equal(t, "pve01", remote)
}
