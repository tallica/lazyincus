package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func onProfiles(t *testing.T, opts options) *Model {
	t.Helper()

	opts.fixture = opts.server == nil
	m := start(t, opts, 140, 40)
	press(m, "3", "right", "right", "right")
	require.Equal(t, "profiles", m.currentView())

	return m
}

func TestProfilesListWithTheirDevicesTab(t *testing.T) {
	m := onProfiles(t, options{})

	rows := rowsOf(m, "profiles")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "default")
	assert.Contains(t, mainText(m), "eth0")

	press(m, "]")
	assert.Contains(t, mainText(m), "Used by:      2")
}
