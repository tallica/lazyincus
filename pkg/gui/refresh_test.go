package gui

import (
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshSeqTurnsAwayAnOlderFetch(t *testing.T) {
	var seq refreshSeq

	older := seq.issue()
	newer := seq.issue()

	assert.True(t, seq.admit(newer))
	assert.False(t, seq.admit(older))
}

func TestRefreshSeqInvalidateTurnsAwayFetchesInFlight(t *testing.T) {
	var seq refreshSeq

	inFlight := seq.issue()
	seq.invalidate()

	assert.False(t, seq.admit(inFlight))
	assert.True(t, seq.admit(seq.issue()))
}

// A refresh that re-sorts the list keeps the cursor on the same instance,
// not on the same row.
func TestSelectionFollowsAnInstanceThatResorts(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, s.gui.Panels.Instances.HandleNextLine)
	s.settle(t, "Name:         web")

	instances := fixtureServer().Instances
	for i := range instances {
		if instances[i].Name == "web" {
			instances[i].Status = "Stopped"
		}
	}

	s.server.SetInstances(instances)
	require.NoError(t, s.gui.refreshInstances())

	screen := s.settle(t, "Status:       stopped")
	assert.Contains(t, screen, "Name:         web")
}

func TestProjectSwitchShowsOnlyThatProject(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.server.SetInstances(append(fixtureServer().Instances, api.InstanceFull{Instance: api.Instance{
		Name: "api", Project: "other", Status: "Running", Type: "container",
	}}))

	s.do(t, func() error { return s.gui.switchToProject("other") })

	screen := s.settle(t, "(fake/other)")
	assert.Contains(t, screen, "│api ")
	assert.NotContains(t, screen, "│web ")
}

// A services fetch fails while the compose project isn't deployed; the
// instances fetched alongside it still land.
func TestAFailedFetchDoesNotHoldBackTheOthers(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	applied := make(chan struct{})
	succeeds := func() (func() error, error) {
		return func() error { close(applied); return nil }, nil
	}
	fails := func() (func() error, error) { return nil, errors.New("project not found") }

	thenRan := false
	err := s.gui.refresh(func() error { thenRan = true; return nil }, fails, succeeds)
	require.EqualError(t, err, "project not found")

	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		t.Fatal("the fetch that succeeded was never applied")
	}

	s.do(t, func() error {
		assert.False(t, thenRan)
		return nil
	})
}

func TestProjectSwitchForgetsTheSnapshotsShown(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "Snapshots (a-name-long")

	s.do(t, func() error { return s.gui.switchToProject("other") })

	screen := s.settle(t, "(fake/other)")
	assert.NotContains(t, screen, "Snapshots (a-name-long")
}

// A volume of the same key in the new scope isn't the one the panel was
// following: nothing is, until a list hands its selection over again.
func TestProjectSwitchForgetsTheVolumeFollowed(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Volumes) })
	s.settle(t, "before-migration")
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Profiles) })

	s.do(t, func() error { return s.gui.switchToProject("default") })

	assert.NotContains(t, s.settle(t, "(fake/default)"), "before-migration")
}

// The switch blanks the focused panel before the new project's instances
// arrive; the same instance at the top must still redraw the main panel.
func TestProjectSwitchRedrawsTheSameFirstInstance(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "Name:         a-name-long-enough-to-be-cut-off")

	s.do(t, func() error { return s.gui.switchToProject("default") })

	screen := s.settle(t, "(fake/default)")
	assert.NotContains(t, screen, "No instances")
	assert.Contains(t, screen, "Name:         a-name-long-enough-to-be-cut-off")
}

func TestUnreachableDaemonIsReportedAndRecovers(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.server.SetDown(true)
	require.NoError(t, s.gui.refreshInstancesQuiet())

	screen := s.settle(t, s.gui.Tr.ConnectionLostTitle)
	assert.Contains(t, screen, "✗")

	s.server.SetDown(false)
	require.NoError(t, s.gui.refreshInstancesQuiet())

	screen = s.settle(t, "●")
	assert.NotContains(t, screen, s.gui.Tr.ConnectionLostTitle)
}

// The stack's instances have rows only in the services panel, so the
// instances poll keeps it current too - an address arriving, a replica
// started from a shell.
func TestTheInstancesPollRefreshesTheServices(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)

		// No stream, so no catch-up refreshing the services instead.
		t.Cleanup(s.server.HoldListen())
	})

	webInstances := func() int {
		count := -1
		s.do(t, func() error {
			for _, row := range s.gui.Panels.Services.List.GetAllItems() {
				if row.Service.Name == "web" {
					count = len(row.Service.Instances)
				}
			}

			return nil
		})

		return count
	}
	require.Eventually(t, func() bool { return webInstances() == 0 }, 5*time.Second, 20*time.Millisecond)

	instances := fixtureServer().Instances
	for i := range instances {
		if instances[i].Name == "web" {
			instances[i].ExpandedConfig = maps.Clone(instances[i].ExpandedConfig)
			instances[i].ExpandedConfig["user.label.incus-compose.service"] = "web"
		}
	}
	s.server.SetInstances(instances)

	require.NoError(t, s.gui.refreshInstancesQuiet())
	require.Eventually(t, func() bool { return webInstances() == 1 }, 3*time.Second, 20*time.Millisecond)
}

// A slow listing holds back only its own panel, at startup and on a
// change of scope, which says it's loading rather than that there's
// nothing to list.
func TestASlowPanelLoadsOnItsOwn(t *testing.T) {
	var release func()

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		release = s.server.HoldImages()
	})
	t.Cleanup(func() { release() })

	screen := s.settle(t, "Name:         a-name-long-enough-to-be-cut-off")
	assert.Contains(t, screen, "│Loading…")
	assert.NotContains(t, screen, "Alpine 3.22 amd64 2")

	release()
	s.ready(t)

	release = s.server.HoldImages()
	s.do(t, func() error { return s.gui.switchToProject("default") })

	// Nothing polls the networks here: only the switch's own read answers them.
	require.Eventually(t, func() bool {
		return strings.Contains(onLoop(t, s, s.gui.Views.Networks.Buffer), "incusbr0")
	}, 5*time.Second, 20*time.Millisecond)
	assert.Contains(t, s.snapshot(t), "│Loading…")

	release()

	screen = s.settle(t, "Alpine 3.22 amd64 2")
	assert.NotContains(t, screen, "Loading…")
}
