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

func TestSnapshotsFollowACustomVolume(t *testing.T) {
	m := onVolumes(t, options{})

	assert.Equal(t, "Snapshots (data)", m.snapshotsTitle())
	rows := rowsOf(m, "snapshots")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "before-migration")

	press(m, "j")
	assert.Equal(t, "Snapshots (data)", m.snapshotsTitle(), "an instance's volume leaves the panel where it was")
}

func TestAVolumeSnapshotsConfigTabNamesItsVolume(t *testing.T) {
	m := onVolumes(t, options{})

	press(m, "2")
	assert.Contains(t, mainText(m), "Volume:       data (default)")
	assert.NotContains(t, mainText(m), "Stateful:")
}

func TestRenamingAVolumesSnapshotFollowsIt(t *testing.T) {
	m := onVolumes(t, options{})
	press(m, "2")

	run(m, onSnapshot((*Model).snapshotRename)(m))
	assert.Equal(t, "Rename snapshot before-migration of data", m.topPrompt().title)
	press(m, "ctrl+u")
	typeText(m, "baseline")
	press(m, "enter")

	assert.Equal(t, "baseline", m.selectedSnapshot().Name)
}

func TestNOnAVolumeOffersNoStatefulField(t *testing.T) {
	m := onVolumes(t, options{})

	press(m, "n")
	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "New snapshot of data", m.topPrompt().title)
	assert.Equal(t, 1, m.topPrompt().snapshot.fieldCount())

	press(m, "esc", "j", "n")
	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.CannotSnapshotInstanceVolume, ansi.Strip(m.popups[0].message))
}
