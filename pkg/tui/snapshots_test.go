package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/utils"
)

func snapshotRows(m *Model) []string {
	p := m.panelNamed("snapshots")
	rows := make([]string, len(p.rows))
	for i, row := range p.rows {
		rows[i] = strings.Join(strings.Fields(utils.Decolorise(row)), " ")
	}

	return rows
}

func TestSnapshotsFollowTheSelectedInstanceNewestFirst(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	assert.Empty(t, snapshotRows(m), "a-name-long-enough-to-be-cut-off has none")
	press(m, "j")
	require.Equal(t, "web", m.selectedInstance().Name)
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())

	rows := snapshotRows(m)
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "daily-b "), rows[0])
	assert.True(t, strings.HasPrefix(rows[1], "daily-a "), rows[1])
}

func TestSnapshotsKeepFollowingWhileTheirOwnListHasFocus(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "j", "2", "j")
	assert.Equal(t, "snapshots", m.currentView())
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())
	assert.Equal(t, "daily-a", m.selectedSnapshot().Name)

	refreshNow(m)
	assert.Equal(t, "daily-a", m.selectedSnapshot().Name, "the selection follows its snapshot across a refresh")
}

func TestESnapshotsListsEveryInstancesWithItsOwner(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "2", "e")
	assert.Equal(t, "Snapshots (all)", m.snapshotsTitle())
	rows := snapshotRows(m)
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "daily-b web "), rows[0])

	press(m, "e")
	assert.Equal(t, "Snapshots (a-name-long-enough-to-be-cut-off)", m.snapshotsTitle(), "back to Instances' selection")
	assert.Empty(t, snapshotRows(m))
}

func TestTheSnapshotConfigTabNamesItsInstance(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "j", "2")
	text := mainText(m)
	assert.Contains(t, text, "Instance:     web")
	assert.Contains(t, text, "Name:         daily-b")
	assert.Contains(t, text, "Full details:")
}
