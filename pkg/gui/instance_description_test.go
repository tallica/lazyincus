package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func editDescription(t *testing.T, s *screen, description string) {
	t.Helper()

	s.pressKey(t, tcell.KeyCtrlP)
	s.settle(t, "Commands")
	s.typeText(t, "edit description")
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "Description of web")

	s.pressKey(t, tcell.KeyCtrlU)
	s.typeText(t, description)
	s.pressKey(t, tcell.KeyEnter)
}

// The description is set from the palette, and cleared by saving nothing.
func TestEditAnInstancesDescription(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	selectInstance(t, s, "web")

	editDescription(t, s, "Shop front")
	assert.Regexp(t, `Description: +Shop front`, s.settle(t, "Shop front"))

	editDescription(t, s, "")
	require.Eventually(t, func() bool {
		return !strings.Contains(s.settle(t, "Name:"), "Description:")
	}, 5*time.Second, 50*time.Millisecond)
}
