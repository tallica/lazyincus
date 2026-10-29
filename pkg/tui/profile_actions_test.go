package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheDefaultProfileIsNeverOfferedForDeletion(t *testing.T) {
	m := onProfiles(t, options{})

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.CannotDeleteDefaultProfile, ansi.Strip(m.popups[0].message))
}

func TestUOnAProfileNarrowsInstancesToWhatHasIt(t *testing.T) {
	m := onProfiles(t, options{})

	press(m, "u")
	assert.Equal(t, fmt.Sprintf(m.tr.InstancesUsing, "default"), m.instancesTitle())
	assert.Len(t, shownInstances(m), 3, "named by the profile, or naming it")
}

func TestProfileKeysAreRefusedReadOnly(t *testing.T) {
	m := onProfiles(t, options{readOnly: true})

	for _, key := range []string{"c", "d"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}
