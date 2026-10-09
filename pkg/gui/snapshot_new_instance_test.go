package gui

import (
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInstanceFromSnapshot(t *testing.T, s *screen) {
	t.Helper()

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Snapshots) })
	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "new instance from snapshot")
	s.pressKey(t, tcell.KeyEnter)
}

// The CLI copies the snapshot into a new instance, named in the prompt,
// in the source's project, and the instance list shows it once it's done,
// with the cursor on it.
func TestNewInstanceFromASnapshot(t *testing.T) {
	ran := make(chan []string, 1)

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.runIncus = func(cmd *exec.Cmd) (string, error) {
			copied := fixtureServer().Instances[1]
			copied.Name, copied.Status, copied.Snapshots = "web-daily-b", "Stopped", nil
			s.server.SetInstances(append(fixtureServer().Instances, copied))
			ran <- cmd.Args

			return "", nil
		}
	})
	s.ready(t)
	selectInstance(t, s, "web")
	s.settle(t, "daily-a")

	newInstanceFromSnapshot(t, s)
	s.settle(t, "New instance from web/daily-b")
	s.pressKey(t, tcell.KeyEnter)

	select {
	case args := <-ran:
		assert.Equal(t, []string{"incus", "--project", "default", "copy", "web/daily-b", "web-daily-b"}, args)
	case <-time.After(5 * time.Second):
		t.Fatal("incus copy never ran")
	}

	require.Eventually(t, func() bool {
		return slices.Contains(instanceNames(t, s), "fake:web-daily-b")
	}, 5*time.Second, 20*time.Millisecond)

	s.settle(t, "Snapshots (web-daily-b)")
	assert.Equal(t, "instances", onLoop(t, s, s.gui.currentViewName))
}

// What the CLI says when it fails is the error.
func TestNewInstanceFromASnapshotFails(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.runIncus = func(*exec.Cmd) (string, error) {
			return "", errors.New("Error: Instance name \"web-daily-b\" already in use")
		}
	})
	s.ready(t)
	selectInstance(t, s, "web")
	s.settle(t, "daily-a")

	newInstanceFromSnapshot(t, s)
	s.settle(t, "New instance from web/daily-b")
	s.pressKey(t, tcell.KeyEnter)

	s.settle(t, "already in use")
}

func TestNewInstanceFromAVolumeSnapshotIsRefused(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Volumes) })
	s.settle(t, "before-migration")

	newInstanceFromSnapshot(t, s)
	s.settle(t, "Only an instance's snapshot")
}

// A compose instance's copy would join its service.
func TestNewInstanceFromAComposeSnapshotIsRefused(t *testing.T) {
	api1 := composeFixture("default", "api-1", "api")
	api1.Snapshots = []api.InstanceSnapshot{{Name: "api-1/nightly"}}

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.server.SetInstances([]api.InstanceFull{api1})
	})
	s.ready(t)
	selectInstance(t, s, "api-1")
	s.settle(t, "nightly")

	newInstanceFromSnapshot(t, s)
	s.settle(t, "api-1 belongs to a compose stack")
}
