package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func TestThePollRaisesAndClosesTheConnectionPopup(t *testing.T) {
	server := incustest.New(incustest.Server{Version: "7.4"})
	m := start(t, options{server: server}, 90, 16)

	server.SetDown(true)
	run(m, m.pollInstances())
	require.Len(t, m.popups, 1)
	assert.Contains(t, strings.Join(screen(m), "\n"), "Connection lost")
	assert.Contains(t, footer(m), "✗")

	run(m, m.pollInstances())
	assert.Len(t, m.popups, 1, "raised once per drop")

	server.SetDown(false)
	run(m, m.pollInstances())
	assert.Empty(t, m.popups)
	assert.Contains(t, footer(m), "●")
}

func TestTheConnectionPopupWaitsOutAnotherPopup(t *testing.T) {
	server := incustest.New(incustest.Server{Version: "7.4"})
	m := start(t, options{server: server}, 90, 16)

	press(m, "x")
	server.SetDown(true)
	run(m, m.pollInstances())
	require.Len(t, m.popups, 1)
	assert.Equal(t, menuView, m.popups[0].view)

	press(m, "esc")
	run(m, m.pollInstances())
	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.ConnectionLostTitle, m.popups[0].title)
}

func TestAnEditedConfigIsReloaded(t *testing.T) {
	m := start(t, options{}, 90, 16)
	before := m.theme.activeBorder

	filename := m.cfg.ConfigFilename()
	require.NoError(t, os.WriteFile(filename, []byte("gui:\n  theme:\n    activeBorderColor: [red]\n"), 0o600))
	m.Update(configStatMsg{modTime: time.Unix(1, 0)})
	assert.Equal(t, before, m.theme.activeBorder, "the first look only notes the time")

	m.Update(configStatMsg{modTime: time.Unix(2, 0)})
	assert.NotEqual(t, before, m.theme.activeBorder)
}
