package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

// messages runs cmd and what a batch of it holds, the way the program would.
func messages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}

	var msgs []tea.Msg
	for _, cmd := range batch {
		msgs = append(msgs, messages(cmd)...)
	}

	return msgs
}

// With no clipboard tool, as over SSH, the copy goes through the terminal.
func TestACopyWithNoClipboardCommandGoesThroughTheTerminal(t *testing.T) {
	m := start(t, options{}, 90, 16)
	t.Setenv("PATH", t.TempDir())

	copied := m.copyToClipboard("cache")()
	require.ErrorIs(t, copied.(copiedMsg).err, commands.ErrNoClipboardCommand)

	_, cmd := m.Update(copied)
	assert.Contains(t, messages(cmd), tea.SetClipboard("cache")())
	assert.Regexp(t, `^sent to the terminal's clipboard: cache `, footer(m))
}

// A configured command that fails still says so, rather than falling back.
func TestAFailingClipboardCommandShowsItsError(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) { c.OS.CopyToClipboardCommand = "false" }}, 90, 16)

	copied := m.copyToClipboard("cache")()
	require.Error(t, copied.(copiedMsg).err)

	_, cmd := m.Update(copied)
	assert.NotContains(t, messages(cmd), tea.SetClipboard("cache")())
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, copied.(copiedMsg).err.Error())
}
