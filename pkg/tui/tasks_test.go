package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/utils"
)

func mainText(m *Model) string {
	return utils.Decolorise(strings.Join(m.main.lines, "\n"))
}

func TestAWriteFromAReplacedTaskIsDropped(t *testing.T) {
	m := start(t, options{}, 90, 16)
	old := m.mainTask.generation

	press(m, "]", "]")
	require.Contains(t, mainText(m), "name: cache")

	m.Update(mainWriteMsg{generation: old, number: 9, content: "stale"})
	assert.NotContains(t, mainText(m), "stale")
}

func TestAWriteOlderThanOneAppliedIsDropped(t *testing.T) {
	m := start(t, options{}, 90, 16)
	generation := m.mainTask.generation

	m.Update(mainWriteMsg{generation: generation, number: 3, kind: writeRerender, content: "third"})
	m.Update(mainWriteMsg{generation: generation, number: 2, kind: writeRerender, content: "second"})
	assert.Equal(t, "third", mainText(m))
}

func TestATasksFirstWriteSetsMainUp(t *testing.T) {
	m := start(t, options{}, 90, 16)

	m.mainTask = mainTask{generation: m.mainTask.generation + 1, key: m.mainTask.key}
	m.Update(mainWriteMsg{generation: m.mainTask.generation, number: 1, autoscroll: true, content: numbered(40)})

	assert.True(t, m.main.autoscroll)
	assert.False(t, m.main.wrap)
	assert.Contains(t, m.main.visible(m.main.height), "39")
}

func TestMainKeepsItsTaskWhileItsKeyHolds(t *testing.T) {
	m := start(t, options{}, 90, 16)
	generation := m.mainTask.generation

	refreshNow(m)
	press(m, "/", "esc")
	assert.Equal(t, generation, m.mainTask.generation)
}

func TestTasksRunThroughPkgTasksAndTheInbox(t *testing.T) {
	m := start(t, options{}, 90, 16)
	m.runTask = nil
	t.Cleanup(m.tasks.Close)

	press(m, "]", "]")
	// The Logs tab's writes may come through first, and be dropped.
	for range 10 {
		if deliver(t, m); strings.Contains(mainText(m), "name: cache") {
			break
		}
	}
	assert.Contains(t, mainText(m), "name: cache")
}
