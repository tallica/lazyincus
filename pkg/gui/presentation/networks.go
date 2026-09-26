package presentation

import (
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetNetworkDisplayStrings(network *commands.Network, showProject bool) []string {
	cells := []string{
		network.Name,
		utils.ColoredString(network.Network.Type, color.FgMagenta),
		displayNetworkManaged(network),
		utils.ColoredString(strconv.Itoa(network.UsedByCount()), color.FgYellow),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(network.Network.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// displayNetworkManaged marks the networks Incus controls; the rest are
// host interfaces it merely knows about, and can't be edited or deleted.
func displayNetworkManaged(network *commands.Network) string {
	if network.IsManaged() {
		return utils.ColoredString("managed", color.FgGreen)
	}

	return "unmanaged"
}

// GetNetworkLeaseRows lays leases out one host to a row, under a header. The
// daemon lists an entry per address, which would give a dual-stack instance
// two rows saying half each. The network's own addresses lead.
func GetNetworkLeaseRows(leases []api.NetworkLease) [][]string {
	type host struct {
		name, hwaddr, kind string
		ipv4, ipv6         []string
	}

	hosts := []*host{}

	for _, lease := range leases {
		entry, ok := lo.Find(hosts, func(h *host) bool {
			return h.name == lease.Hostname && h.hwaddr == lease.Hwaddr
		})
		if !ok {
			entry = &host{name: lease.Hostname, hwaddr: lease.Hwaddr, kind: strings.ToLower(lease.Type)}
			hosts = append(hosts, entry)
		}

		if ip := net.ParseIP(lease.Address); ip != nil && ip.To4() == nil {
			entry.ipv6 = append(entry.ipv6, lease.Address)
		} else {
			entry.ipv4 = append(entry.ipv4, lease.Address)
		}
	}

	slices.SortStableFunc(hosts, func(a, b *host) int {
		if (a.kind == "gateway") != (b.kind == "gateway") {
			if a.kind == "gateway" {
				return -1
			}

			return 1
		}

		return strings.Compare(a.name, b.name)
	})

	rows := make([][]string, 0, 1+len(hosts))
	rows = append(rows, []string{"HOSTNAME", "IPV4", "IPV6", "MAC", "TYPE"})

	for _, h := range hosts {
		rows = append(rows, []string{
			h.name,
			utils.ColoredString(strings.Join(h.ipv4, ", "), color.FgGreen),
			utils.ColoredString(strings.Join(h.ipv6, ", "), color.FgGreen),
			utils.ColoredString(h.hwaddr, color.FgCyan),
			displayLeaseType(h.kind),
		})
	}

	return rows
}

// GetACLRuleRows lays out one direction's rules under a header, in the order
// the ACL has them.
func GetACLRuleRows(rules []api.NetworkACLRule) [][]string {
	rows := make([][]string, 0, 1+len(rules))
	rows = append(rows, []string{"ACTION", "SOURCE", "DESTINATION", "PROTOCOL", "PORT", "STATE"})

	for _, rule := range rules {
		protocol := rule.Protocol
		if rule.ICMPType != "" {
			protocol += " type " + rule.ICMPType
		}

		port := rule.DestinationPort
		if rule.SourcePort != "" {
			port = strings.TrimSpace(port + " from " + rule.SourcePort)
		}

		rows = append(rows, []string{
			DisplayACLAction(rule.Action),
			orAny(rule.Source),
			orAny(rule.Destination),
			orAny(protocol),
			port,
			displayACLState(rule.State),
		})
	}

	return rows
}

// GetNetworkForwardRows lays out the network's forwards a port range to a
// row, under a header, the listen address repeated so each row stands on
// its own. A forward's target_address takes the ports no entry names.
// owner names the instance at an address inside, or "".
func GetNetworkForwardRows(forwards []api.NetworkForward, owner func(address string) string) [][]string {
	rows := [][]string{{"LISTEN", "PROTOCOL", "PORT", "TARGET", "INSTANCE", "DESCRIPTION"}}

	target := func(address, port string) string {
		if port == "" {
			return address
		}

		return net.JoinHostPort(address, port)
	}

	for _, forward := range forwards {
		for _, port := range forward.Ports {
			targetPort := port.TargetPort
			if targetPort == "" {
				targetPort = port.ListenPort
			}

			rows = append(rows, []string{
				utils.ColoredString(forward.ListenAddress, color.FgYellow),
				port.Protocol,
				port.ListenPort,
				utils.ColoredString(target(port.TargetAddress, targetPort), color.FgGreen),
				utils.ColoredString(owner(port.TargetAddress), color.FgCyan),
				port.Description,
			})
		}

		if fallback := forward.Config["target_address"]; fallback != "" {
			rows = append(rows, []string{
				utils.ColoredString(forward.ListenAddress, color.FgYellow),
				"any", "other",
				utils.ColoredString(fallback, color.FgGreen),
				utils.ColoredString(owner(fallback), color.FgCyan),
				forward.Description,
			})
		}
	}

	return rows
}

// DisplayACLAction colours what a rule does to the traffic it matches.
func DisplayACLAction(action string) string {
	switch action {
	case "allow", "allow-stateless":
		return utils.ColoredString(action, color.FgGreen)
	case "reject", "drop":
		return utils.ColoredString(action, color.FgRed)
	default:
		return action
	}
}

// displayACLState dims a rule that's switched off; "logged" is enabled and
// then some.
func displayACLState(state string) string {
	if state == "disabled" {
		return utils.ColoredString(state, color.FgHiBlack)
	}

	return state
}

func orAny(value string) string {
	if value == "" {
		return "any"
	}

	return value
}

// displayLeaseType sets apart the addresses that aren't DHCP's to give out:
// the network's own, and those pinned in an instance's config.
func displayLeaseType(kind string) string {
	switch kind {
	case "gateway":
		return utils.ColoredString(kind, color.FgMagenta)
	case "static":
		return utils.ColoredString(kind, color.FgYellow)
	default:
		return kind
	}
}
