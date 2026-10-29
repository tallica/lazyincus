package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestMainPassesKeysItDoesntTakeToItsPanel(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "enter", "]")
	assert.Equal(t, mainView, m.currentView())
	assert.Equal(t, 1, m.panelNamed("instances").mainTab)

	press(m, "j")
	assert.Equal(t, 0, m.panelNamed("instances").selected, "main's own j scrolls rather than moving the list")
}

// gocui forgets main's parent once the search prompt has had focus.
func TestMainForgetsItsPanelAfterASearch(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "enter", "/", "x", "enter")
	require.Equal(t, mainView, m.currentView())
	press(m, "]")
	assert.Equal(t, 0, m.panelNamed("instances").mainTab)
}

func TestAMutatingKeyFromMainIsGuardedToo(t *testing.T) {
	m := start(t, options{readOnly: true}, 140, 40)

	press(m, "enter", "E")
	require.Len(t, m.popups, 1)
	assert.Contains(t, strings.Join(screen(m), "\n"), "--read-only")
}

func TestAReadOnlyRemoteRefusesNamingIt(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}}, 140, 40)

	press(m, "E")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "fake is read-only in the config")
}

func TestAMessageClosesOnEveryConfirmationKey(t *testing.T) {
	for _, key := range []string{"enter", "y", "esc", "n"} {
		m := start(t, options{}, 140, 40)
		m.openError("boom")

		press(m, key)
		assert.Empty(t, m.popups, key)
	}
}

func TestGlobalKeysReachAPopup(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openError("boom")

	press(m, "3")
	assert.Empty(t, m.popups, "focusing a panel drops the popup, as gocui's switchFocus does")
	assert.Equal(t, "images", m.currentView())
}

func TestTheOptionsListNamesOnlyKeysItCanLabel(t *testing.T) {
	m := start(t, options{}, 140, 40)

	labels := map[string]bool{}
	for _, b := range m.bindings() {
		if b.listed() {
			labels[b.label()] = true
		}
	}

	assert.True(t, labels["◄"])
	assert.True(t, labels["E"])
	assert.False(t, labels[""], "home and end have no label")
}
