package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func update(m *Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	run(m, cmd)
}

// Away, the instances poll waits a minute between fetches; focus catches up.
func TestPollsSlowWhileAwayAndCatchUpOnFocus(t *testing.T) {
	server := instancesServer(nil, runningInstance("web", nil))
	m := start(t, options{server: server}, 90, 16)

	update(m, tea.BlurMsg{})
	update(m, pollMsg{})
	server.SetInstances(append(server.Instances, runningInstance("cache", nil)))

	update(m, pollMsg{})
	assert.Empty(t, rowOf(m, "cache"), "a poll inside the minute fetches nothing")

	update(m, tea.FocusMsg{})
	assert.NotEmpty(t, rowOf(m, "cache"))

	server.SetInstances(server.Instances[:1])
	update(m, pollMsg{})
	assert.Empty(t, rowOf(m, "cache"), "focused, every poll fetches")
}

// Main's ticking tab stops while away, and starts again on focus.
func TestMainsTaskRestartsOnFocus(t *testing.T) {
	m := start(t, options{}, 90, 16)
	generation := m.mainTask.generation

	update(m, tea.BlurMsg{})
	assert.Equal(t, generation, m.mainTask.generation)

	update(m, tea.FocusMsg{})
	assert.Equal(t, generation+1, m.mainTask.generation)
}

// A terminal reporting focus without having lost it changes nothing.
func TestFocusWithoutABlurDoesNothing(t *testing.T) {
	m := start(t, options{}, 90, 16)
	generation := m.mainTask.generation

	_, cmd := m.Update(tea.FocusMsg{})
	assert.Nil(t, cmd)
	assert.Equal(t, generation, m.mainTask.generation)
}
