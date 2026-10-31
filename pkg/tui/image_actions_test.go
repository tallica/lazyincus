package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletingAnImageAsksThenDropsIt(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	press(m, "3", "j")

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to delete image Debian 13 amd64?", ansi.Strip(m.popups[0].message))
	press(m, "y")
	assert.Len(t, rowsOf(m, "images"), 1)
}

func TestPruningNamesEveryImageItWouldDelete(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	press(m, "3", "D")

	require.NotNil(t, m.topMenu())
	labels := strings.Join(screen(m), "\n")
	assert.Contains(t, labels, "unused cached images (1, 90.00MiB)")
	assert.Contains(t, labels, "every unused image (1, 90.00MiB)")

	press(m, "j", "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "Debian 13 amd64 fedcba987654")
	press(m, "y")
	assert.Len(t, rowsOf(m, "images"), 1)
}

func TestImageKeysAreRefusedReadOnly(t *testing.T) {
	m := start(t, options{fixture: true, readOnly: true}, 140, 40)
	press(m, "3")

	for _, key := range []string{"d", "c", "D"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}

func TestUNarrowsInstancesToAnImagesUsersUntilEsc(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	press(m, "3")

	press(m, "u")
	assert.Equal(t, "instances", m.currentView())
	assert.Equal(t, fmt.Sprintf(m.tr.InstancesUsing, "Alpine 3.22 amd64"), m.instancesTitle())
	assert.ElementsMatch(t, []string{"default/web", "default/a-name-long-enough-to-be-cut-off"}, shownInstances(m))

	press(m, "esc")
	assert.Equal(t, "images", m.currentView())
	assert.Equal(t, m.tr.InstancesTitle, m.instancesTitle())
	assert.Len(t, shownInstances(m), 3)
}

func TestUOnAnUnusedImageSaysSo(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	press(m, "3", "j", "u")

	require.Len(t, m.popups, 1)
	assert.Equal(t, fmt.Sprintf(m.tr.NothingUses, "Debian 13 amd64"), ansi.Strip(m.popups[0].message))
	assert.Equal(t, m.tr.InstancesTitle, m.instancesTitle())
}
