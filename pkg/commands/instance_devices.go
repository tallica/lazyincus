package commands

import (
	"maps"
	"net"
	"slices"
	"strings"
)

// PublishedPorts is each proxy device as `listen → connect port`, in device
// name order. incus-compose publishes a compose port as one of these,
// listening on the daemon's host and connecting to the instance's own
// loopback, so the connect side's address says nothing and is left off; a
// wildcard listen address is `*`.
func (i *Instance) PublishedPorts() []string {
	var ports []string

	for _, name := range slices.Sorted(maps.Keys(i.Instance.ExpandedDevices)) {
		device := i.Instance.ExpandedDevices[name]
		if device["type"] != "proxy" {
			continue
		}

		protocol, listenHost, listenPort, ok := splitProxyAddress(device["listen"])
		if !ok {
			ports = append(ports, device["listen"])
			continue
		}

		if listenHost == "0.0.0.0" || listenHost == "::" {
			listenHost = "*"
		}

		port := net.JoinHostPort(listenHost, listenPort)
		if _, _, connectPort, ok := splitProxyAddress(device["connect"]); ok {
			port += " → " + connectPort
		}

		if protocol != "tcp" {
			port += "/" + protocol
		}

		ports = append(ports, port)
	}

	return ports
}

// splitProxyAddress takes a proxy device's `tcp:0.0.0.0:80` apart. A unix
// socket has no port, and isn't ok.
func splitProxyAddress(address string) (protocol, host, port string, ok bool) {
	protocol, hostPort, found := strings.Cut(address, ":")
	if !found || protocol == "unix" {
		return "", "", "", false
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", "", "", false
	}

	return protocol, host, port, true
}

// CustomVolumes is the custom storage volume behind each disk device that
// has one, by device name; a host path bound in has no pool, and isn't one.
func (i *Instance) CustomVolumes() map[string]string {
	volumes := map[string]string{}

	for name, device := range i.Instance.ExpandedDevices {
		if device["type"] == "disk" && device["pool"] != "" && device["source"] != "" {
			volumes[name] = device["source"]
		}
	}

	return volumes
}

// NetworkNames is the managed network each nic is attached to, sorted and
// once each.
func (i *Instance) NetworkNames() []string {
	var networks []string

	for _, device := range i.Instance.ExpandedDevices {
		if device["type"] == "nic" && device["network"] != "" {
			networks = append(networks, device["network"])
		}
	}

	slices.Sort(networks)

	return slices.Compact(networks)
}
