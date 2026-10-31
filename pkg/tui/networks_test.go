package tui

import (
	"slices"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"

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

// State reads the newest listing each tick, so a port's instance listed since gets named.
func TestTheNetworkStateTabFollowsTheInstanceList(t *testing.T) {
	server := incustest.Fixture()
	m := onNetworks(t, options{server: server})
	state := m.networkState(m.selectedNetwork())
	require.NotContains(t, state(), "veth99  cache (eth0)")

	cache := runningInstance("cache", nil)
	cache.State = &api.InstanceState{Status: "Running", Network: map[string]api.InstanceStateNetwork{"eth0": {HostName: "veth99"}}}
	server.SetInstances(append(slices.Clone(server.Instances), cache))
	run(m, refresh(m.fetchInstances()))

	assert.Contains(t, state(), "veth99  cache (eth0)")
}

// A re-sort works on a copy: a tab's goroutine may be reading the listing.
func TestRerenderingInstancesLeavesTheListingAlone(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	listed := *m.listed.Load()
	before := slices.Clone(listed)
	require.Equal(t, "db", before[len(before)-1].Name, "stopped sorts last")

	m.incus.MarkInstance("default", "db", "Starting")
	m.rerenderInstances()

	require.NotEqual(t, before, m.instances, "the mark re-sorted the rows")
	assert.Equal(t, before, listed)
}
