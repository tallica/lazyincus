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
