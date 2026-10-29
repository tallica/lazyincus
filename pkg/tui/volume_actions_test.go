package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletingAVolumeAsksOnlyForAnUnusedCustomOne(t *testing.T) {
	m := onVolumes(t, options{})

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to delete volume data?", ansi.Strip(m.popups[0].message))
	press(m, "esc", "j", "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.CannotDeleteManagedVolume, ansi.Strip(m.popups[0].message))
}

func TestUOnAVolumeNarrowsInstancesToWhatAttachesIt(t *testing.T) {
	m := onVolumes(t, options{})

	press(m, "j", "u")
	assert.Equal(t, fmt.Sprintf(m.tr.InstancesUsing, "web"), m.instancesTitle())
	assert.Equal(t, []string{"default/web"}, shownInstances(m))
	press(m, "esc")
	assert.Equal(t, "volumes", m.currentView())
}

func TestVolumeKeysAreRefusedReadOnly(t *testing.T) {
	m := onVolumes(t, options{readOnly: true})

	for _, key := range []string{"c", "d"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}
