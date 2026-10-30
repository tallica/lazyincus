package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The footer under a transient status.
func TestATransientStatusLeadsTheFooter(t *testing.T) {
	m := start(t, options{fixture: true}, 90, 16)

	m.withTransientStatus("Copied", time.Second)
	m.fitViews()
	assert.Equal(t, "Copied PgUp/PgDn: scroll, q: quit, x: menu, ← → ↑ ↓ Incus v7.4 (fake/all projects) ●  test",
		strings.TrimRight(footer(m), " "))

	m.Update(transientDoneMsg{name: "Copied"})
	assert.True(t, strings.HasPrefix(footer(m), "PgUp/PgDn"))
}

func TestAWaitingStatusSpinsUntilItsWorkIsDone(t *testing.T) {
	m := start(t, options{}, 90, 16)
	ticks := 0
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd {
		if _, ok := msg.(spinnerMsg); ok {
			ticks++
		}
		return nil
	}

	release := make(chan struct{})
	cmd := m.withWaitingStatus("Restarting", func() error { <-release; return errors.New("boom") })
	m.fitViews()
	assert.Regexp(t, `^Restarting [|/\\-] `, footer(m))

	m.withWaitingStatus("Stopping", func() error { return nil })
	m.Update(spinnerMsg{})
	assert.Equal(t, 2, ticks, "one spinner for every status, re-armed while one waits")

	close(release)
	run(m, cmd)
	require.Len(t, m.popups, 1)
	assert.Contains(t, strings.Join(screen(m), "\n"), "boom")

	m.Update(waitingDoneMsg{name: "Stopping"})
	m.Update(spinnerMsg{})
	assert.Equal(t, 2, ticks, "stopped once nothing waits")
	assert.NotContains(t, footer(m), "Stopping")
}
