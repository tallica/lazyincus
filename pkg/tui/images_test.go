package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/utils"
)

func rowsOf(m *Model, name string) []string {
	p := m.panelNamed(name)
	rows := make([]string, len(p.rows))
	for i, row := range p.rows {
		rows[i] = strings.Join(strings.Fields(utils.Decolorise(row)), " ")
	}

	return rows
}

func TestImagesListByLabelWithTheirUsers(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "3")
	require.Equal(t, "images", m.currentView())
	rows := rowsOf(m, "images")
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "Alpine 3.22 amd64 2 0123456789ab "), rows[0])
	assert.True(t, strings.HasPrefix(rows[1], "Debian 13 amd64 0 "), rows[1])
}

func TestTheImageConfigTabNamesItsUsers(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "3")
	text := mainText(m)
	assert.Contains(t, text, "Fingerprint:  0123456789abcdef0123456789abcdef")
	assert.Contains(t, text, "Used by:      web, a-name-long-enough-to-be-cut-off (default)")
}

func TestASlowPollWaitsOnAnOpenEventStream(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	assert.NotNil(t, m.slowPoll(refreshImages), "no stream: every tick asks")
	assert.NotNil(t, m.slowPoll(refreshImages))

	// The test's ticks are nil, so a poll that only re-arms is nil.
	m.eventsLive = true
	assert.Nil(t, m.slowPoll(refreshImages), "asked within the minute: only re-arms")
}
