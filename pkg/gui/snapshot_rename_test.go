package gui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

// renameSelectedSnapshot runs rename from the palette with Snapshots
// focused, replacing the name with name.
func renameSelectedSnapshot(t *testing.T, s *screen, prompt, name string) {
	t.Helper()

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Snapshots) })
	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "rename snapshots")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, prompt)

	s.pressKey(t, tcell.KeyCtrlU)
	s.typeText(t, name)
	s.pressKey(t, tcell.KeyEnter)
}

func selectedSnapshot(t *testing.T, s *screen) string {
	t.Helper()

	return onLoop(t, s, func() string {
		snapshot, _ := s.gui.Panels.Snapshots.GetSelectedItem()
		return snapshot.Name
	})
}

// An instance's snapshot is renamed while the instance runs, and the cursor
// follows it.
func TestRenameAnInstancesSnapshot(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	selectInstance(t, s, "web")
	s.settle(t, "daily-a")

	renameSelectedSnapshot(t, s, "Rename snapshot daily-b of web", "weekly")

	s.settle(t, "weekly")
	assert.Equal(t, "weekly", selectedSnapshot(t, s))
}

func TestRenameAVolumesSnapshot(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Volumes) })
	s.settle(t, "before-migration")

	renameSelectedSnapshot(t, s, "Rename snapshot before-migration of data", "baseline")

	s.settle(t, "baseline")
	assert.Equal(t, "baseline", selectedSnapshot(t, s))
}
