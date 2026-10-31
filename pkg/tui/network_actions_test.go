package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletingAnUnusedNetworkAsks(t *testing.T) {
	m := onNetworks(t, options{})

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to delete network incusbr0?", ansi.Strip(m.popups[0].message))
}

func TestTheHostsInterfacesCanNeitherBeEditedNorDeleted(t *testing.T) {
	m := onNetworks(t, options{})
	press(m, "e", "k")
	require.Equal(t, "eth0", m.selectedNetwork().Name)

	for key, message := range map[string]string{"c": m.tr.CannotEditUnmanagedNetwork, "d": m.tr.CannotDeleteUnmanagedNetwork} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Equal(t, message, ansi.Strip(m.popups[0].message), key)
		press(m, "esc")
	}
}

func TestUOnANetworkNarrowsInstancesToItsNICs(t *testing.T) {
	m := onNetworks(t, options{})

	press(m, "u")
	assert.Equal(t, fmt.Sprintf(m.tr.InstancesUsing, "incusbr0"), m.instancesTitle())
	assert.Contains(t, shownInstances(m), "default/web")
}

func TestYLeavesOutTheAddressesANetworkHasNot(t *testing.T) {
	m := onNetworks(t, options{})

	press(m, "y")
	require.NotNil(t, m.topMenu())
	assert.Len(t, m.topMenu().items, 2, "the name and cancel")
	assert.Contains(t, strings.Join(screen(m), "\n"), "name   incusbr0")
}

func TestNetworkKeysAreRefusedReadOnly(t *testing.T) {
	m := onNetworks(t, options{readOnly: true})

	for _, key := range []string{"c", "d"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}
