package commands

import (
	"maps"
	"net"
	"slices"
	"strings"
)

// PublishedPort is one proxy device: Port on the daemon's host, forwarded
// to Target inside the instance. Host is where it's reached from here, empty
// for a wildcard listen address with no host to stand for it. A listen
// address that isn't a network one - a unix socket - is kept whole in Raw.
type PublishedPort struct {
	Host     string
	Port     string
	Target   string
	Protocol string
	Raw      string
}

// Label is the port as `host:port → target`, a wildcard with no host
// reading `*`, the protocol added when it isn't tcp.
func (p PublishedPort) Label() string {
	if p.Raw != "" {
		return p.Raw
	}

	host := p.Host
	if host == "" {
		host = "*"
	}

	label := net.JoinHostPort(host, p.Port)
	if p.Target != "" {
		label += " → " + p.Target
	}

	if p.Protocol != "tcp" {
		label += "/" + p.Protocol
	}

	return label
}

// Address is where the port is reached from here, or empty when that isn't
// known.
func (p PublishedPort) Address() string {
	if p.Raw != "" || p.Host == "" {
		return ""
	}

	return net.JoinHostPort(p.Host, p.Port)
}

// PublishedPorts is each proxy device, in device name order. incus-compose
// publishes a compose port as one of these, listening on the daemon's host
// and connecting to the instance's own loopback, so the connect side's
// address says nothing and only its port is kept. A wildcard listen address
// takes host, the remote's own.
func (i *Instance) PublishedPorts(host string) []PublishedPort {
	var ports []PublishedPort

	for _, name := range slices.Sorted(maps.Keys(i.Instance.ExpandedDevices)) {
		device := i.Instance.ExpandedDevices[name]
		if device["type"] != "proxy" {
			continue
		}

		protocol, listenHost, listenPort, ok := splitProxyAddress(device["listen"])
		if !ok {
			ports = append(ports, PublishedPort{Raw: device["listen"]})
			continue
		}

		if listenHost == "0.0.0.0" || listenHost == "::" {
			listenHost = host
		}

		port := PublishedPort{Host: listenHost, Port: listenPort, Protocol: protocol}
		if _, _, connectPort, ok := splitProxyAddress(device["connect"]); ok {
			port.Target = connectPort
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
