package gui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

// selectInstance puts the instances panel's cursor on name.
func selectInstance(t *testing.T, s *screen, name string) {
	t.Helper()

	s.do(t, func() error {
		if !s.gui.Panels.Instances.Select(&commands.Instance{Name: name, Project: "default"}) {
			t.Fatalf("no instance %s", name)
		}

		return s.gui.Panels.Instances.HandleSelect()
	})
}

// Rename has no key: the palette runs it, and the cursor follows the
// instance to where its new name sorts.
func TestThePaletteRenamesAStoppedInstance(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	selectInstance(t, s, "db")

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "rename")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Rename db")

	s.pressKey(t, tcell.KeyBackspace2)
	s.pressKey(t, tcell.KeyBackspace2)
	s.typeText(t, "cache")
	s.pressKey(t, tcell.KeyEnter)

	s.settle(t, "Snapshots (cache)")
	assert.Equal(t, "cache", onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Name
	}))
}

func TestRenameRefusesARunningInstance(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	selectInstance(t, s, "web")

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "rename")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Stop web first")
}

// The keybinding menu lists rename with no key beside it.
func TestTheMenuListsAnActionWithNoKey(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.press(t, 'x')
	assert.Regexp(t, `│ +rename +[│▐]`, s.settle(t, "rename"))
}
