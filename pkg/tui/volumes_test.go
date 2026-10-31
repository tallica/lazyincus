package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onVolumes has the volumes list focused, its first row data.
func onVolumes(t *testing.T, opts options) *Model {
	t.Helper()

	opts.fixture = opts.server == nil
	m := start(t, opts, 140, 40)
	press(m, "3", "right")
	require.Equal(t, "volumes", m.currentView())

	return m
}

func TestVolumesListCustomOnesFirstWithTheirSize(t *testing.T) {
	m := onVolumes(t, options{})

	rows := rowsOf(m, "volumes")
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "data 0 512.00MiB"), rows[0])
	assert.True(t, strings.HasPrefix(rows[1], "web 1 "), rows[1])
}

func TestTheVolumeConfigTabNamesItsPool(t *testing.T) {
	m := onVolumes(t, options{})

	text := mainText(m)
	assert.Contains(t, text, "Name:         data")
	assert.Contains(t, text, "Pool space:")
	assert.Contains(t, text, "Used by:      "+m.tr.UsedByNothing)
}
