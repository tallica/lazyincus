package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func labels(ports []PublishedPort) []string {
	labels := make([]string, 0, len(ports))
	for _, port := range ports {
		labels = append(labels, port.Label())
	}

	return labels
}

func withDevices(devices map[string]map[string]string) *Instance {
	return &Instance{Instance: api.InstanceFull{Instance: api.Instance{ExpandedDevices: devices}}}
}

// incus-compose's proxies connect to the instance's loopback, so only the
// port on that side is worth showing.
func TestPublishedPorts(t *testing.T) {
	instance := withDevices(map[string]map[string]string{
		"proxy-9001": {"type": "proxy", "listen": "tcp:0.0.0.0:9001", "connect": "tcp:127.0.0.1:9001"},
		"proxy-8191": {"type": "proxy", "listen": "tcp:0.0.0.0:8191", "connect": "tcp:127.0.0.1:8080"},
		"proxy-dns":  {"type": "proxy", "listen": "udp:192.0.2.1:53", "connect": "udp:127.0.0.1:53"},
		"proxy-v6":   {"type": "proxy", "listen": "tcp:[::]:443", "connect": "tcp:[::1]:443"},
		"proxy-sock": {"type": "proxy", "listen": "unix:/run/app.sock", "connect": "unix:/run/app.sock"},
		"eth0":       {"type": "nic", "network": "incusbr0"},
	})

	assert.Equal(t, []string{
		"*:8191 → 8080",
		"*:9001 → 9001",
		"192.0.2.1:53 → 53/udp",
		"unix:/run/app.sock",
		"*:443 → 443",
	}, labels(instance.PublishedPorts("")))
}

func TestCustomVolumesLeaveOutRootAndBindMounts(t *testing.T) {
	instance := withDevices(map[string]map[string]string{
		"root":      {"type": "disk", "path": "/", "pool": "default"},
		"vol-data":  {"type": "disk", "path": "/data", "pool": "default", "source": "vol-data"},
		"bind-udev": {"type": "disk", "path": "/run/udev", "source": "/run/udev"},
	})

	assert.Equal(t, map[string]string{"vol-data": "vol-data"}, instance.CustomVolumes())
}

func TestNetworkNames(t *testing.T) {
	instance := withDevices(map[string]map[string]string{
		"eth0": {"type": "nic", "network": "net-b"},
		"eth1": {"type": "nic", "network": "net-a"},
		"eth2": {"type": "nic", "network": "net-b"},
		"eth3": {"type": "nic", "nictype": "macvlan", "parent": "en0"},
	})

	assert.Equal(t, []string{"net-a", "net-b"}, instance.NetworkNames())
}

func TestPublishedPortsAtTheRemotesHost(t *testing.T) {
	instance := withDevices(map[string]map[string]string{
		"proxy-80":  {"type": "proxy", "listen": "tcp:0.0.0.0:8080", "connect": "tcp:127.0.0.1:80"},
		"proxy-dns": {"type": "proxy", "listen": "udp:192.0.2.1:53", "connect": "udp:127.0.0.1:53"},
	})

	assert.Equal(t, []string{"192.0.2.5:8080 → 80", "192.0.2.1:53 → 53/udp"}, labels(instance.PublishedPorts("192.0.2.5")))
	assert.Equal(t, []string{"[2001:db8::5]:8080 → 80", "192.0.2.1:53 → 53/udp"}, labels(instance.PublishedPorts("2001:db8::5")))

	// Only a port whose host is known has an address to copy.
	assert.Equal(t, "192.0.2.5:8080", instance.PublishedPorts("192.0.2.5")[0].Address())
	assert.Empty(t, instance.PublishedPorts("")[0].Address())
}
