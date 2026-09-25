package gui

import (
	"errors"
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
	s.settle(t, "incusbr0")

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
	s.settle(t, "incusbr0")

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
	s.settle(t, "incusbr0")

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
	s.settle(t, "incusbr0")

	s.server.SetDown(true)
	require.NoError(t, s.gui.refreshInstancesQuiet())

	screen := s.settle(t, s.gui.Tr.ConnectionLostTitle)
	assert.Contains(t, screen, "✗")

	s.server.SetDown(false)
	require.NoError(t, s.gui.refreshInstancesQuiet())

	screen = s.settle(t, "●")
	assert.NotContains(t, screen, s.gui.Tr.ConnectionLostTitle)
}
