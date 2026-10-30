package tui

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func onNetworks(t *testing.T, opts options) *Model {
	t.Helper()

	opts.fixture = opts.server == nil
	m := start(t, opts, 140, 40)
	press(m, "3", "right", "right")
	require.Equal(t, "networks", m.currentView())

	return m
}

func TestNetworksLeaveTheHostsInterfacesToE(t *testing.T) {
	m := onNetworks(t, options{})

	rows := rowsOf(m, "networks")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "incusbr0")

	press(m, "e")
	assert.Len(t, rowsOf(m, "networks"), 2)
}

func TestTheNetworkTabsInTheirOrder(t *testing.T) {
	m := onNetworks(t, options{})

	press(m, "]")
	assert.Contains(t, mainText(m), "veth10  web (eth0)", "State names a bridge port's instance")

	press(m, "]")
	acls := mainText(m)
	assert.Contains(t, acls, "On the network:         isolate")
	assert.Contains(t, acls, "Unmatched egress:       allow")
	assert.Contains(t, acls, "── isolate ")

	press(m, "]")
	assert.Contains(t, mainText(m), "198.51.100.7")
	assert.Contains(t, mainText(m), "web")

	press(m, "]")
	assert.Contains(t, mainText(m), "Managed:      true")
}

func TestAForwardEventRedrawsTheNetworkTabs(t *testing.T) {
	m := onNetworks(t, options{})
	before := m.networkKey()

	m.Update(eventMsg{stream: m.events.stream, event: commands.Event{Type: api.EventTypeLifecycle, Action: api.EventLifecycleNetworkForwardCreated}})
	assert.NotEqual(t, before, m.networkKey())
}
