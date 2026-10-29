package presentation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

func NetworkLeases(network *commands.Network, tr *i18n.TranslationSet) string {
	if !network.IsManaged() {
		return tr.NoLeasesUnmanaged
	}

	leases, err := network.Leases()
	if err != nil {
		return tr.CannotListLeases + "\n\n" + err.Error()
	}

	if len(leases) == 0 {
		return tr.NoLeases
	}

	table, err := utils.RenderTable(GetNetworkLeaseRows(leases))
	if err != nil {
		return err.Error()
	}

	return table
}

// NetworkState is `incus network info`, with a bridge's ports named by
// the instance on the other end rather than by veth alone.
func NetworkState(network *commands.Network, instances []*commands.Instance, tr *i18n.TranslationSet) string {
	state, err := network.State()
	if err != nil {
		return tr.CannotReadNetworkState + "\n\n" + err.Error()
	}

	padding := 16
	line := func(indent, label, value string) string {
		if value == "" {
			return ""
		}

		return indent + utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := line("", "State", state.State) +
		line("", "Type", state.Type) +
		line("", "MAC address", state.Hwaddr) +
		line("", "MTU", fmt.Sprint(state.Mtu))

	if len(state.Addresses) > 0 {
		output += "\nIP addresses:\n"
		for _, addr := range state.Addresses {
			output += "  " + utils.WithPadding(addr.Family+": ", 7) +
				fmt.Sprintf("%s/%s (%s)\n", addr.Address, addr.Netmask, addr.Scope)
		}
	}

	if counters := state.Counters; counters != nil {
		output += "\nNetwork usage:\n" +
			line("  ", "Received", fmt.Sprintf("%s (%d packets)",
				units.GetByteSizeStringIEC(counters.BytesReceived, 2), counters.PacketsReceived)) +
			line("  ", "Sent", fmt.Sprintf("%s (%d packets)",
				units.GetByteSizeStringIEC(counters.BytesSent, 2), counters.PacketsSent))
	}

	if bridge := state.Bridge; bridge != nil {
		output += "\nBridge:\n" +
			line("  ", "ID", bridge.ID) +
			line("  ", "STP", fmt.Sprint(bridge.STP)) +
			line("  ", "VLAN filtering", fmt.Sprint(bridge.VLANFiltering))

		if len(bridge.UpperDevices) > 0 {
			output += "  Ports:\n"

			owners := hostInterfaceOwners(instances)
			for _, device := range bridge.UpperDevices {
				output += "    " + device
				if owner, ok := owners[device]; ok {
					output += "  " + utils.ColoredString(owner, color.FgCyan)
				}
				output += "\n"
			}
		}
	}

	if bond := state.Bond; bond != nil {
		output += "\nBond:\n" +
			line("  ", "Mode", bond.Mode) +
			line("  ", "MII state", bond.MIIState) +
			line("  ", "Devices", strings.Join(bond.LowerDevices, ", "))
	}

	if vlan := state.VLAN; vlan != nil {
		output += "\nVLAN:\n" +
			line("  ", "Parent", vlan.LowerDevice) +
			line("  ", "ID", fmt.Sprint(vlan.VID))
	}

	if ovn := state.OVN; ovn != nil {
		output += "\nOVN:\n" +
			line("  ", "Chassis", ovn.Chassis) +
			line("  ", "Uplink IPv4", ovn.UplinkIPv4) +
			line("  ", "Uplink IPv6", ovn.UplinkIPv6)
	}

	return output
}

// hostInterfaceOwners names the instance behind each host-side interface,
// as "instance (eth0)", from the newest instance listing.
func hostInterfaceOwners(instances []*commands.Instance) map[string]string {
	owners := map[string]string{}

	for _, instance := range instances {
		state := instance.Latest().Instance.State
		if state == nil {
			continue
		}

		for name, nic := range state.Network {
			if nic.HostName != "" {
				owners[nic.HostName] = fmt.Sprintf("%s (%s)", instance.Name, name)
			}
		}
	}

	return owners
}

// NetworkACLs is what filters the network's traffic: the ACLs on the
// network, what happens to what none of them match, those on single NICs,
// then each ACL's rules. It answers "why can't this reach that", which the
// Config tab's security.acls, a list of names, doesn't.
func NetworkACLs(network *commands.Network, instances []*commands.Instance, width int, tr *i18n.TranslationSet) string {
	if !network.IsManaged() {
		return tr.NoACLsUnmanaged
	}

	onNetwork := network.ACLNames()
	onNICs := network.NICACLs(instances)

	if len(onNetwork) == 0 && len(onNICs) == 0 {
		return tr.NoACLs
	}

	padding := 24
	output := ""

	if len(onNetwork) > 0 {
		unmatched := func(direction string) string {
			action := network.Network.Config["security.acls.default."+direction+".action"]
			if action == "" {
				// The daemon's default for traffic no rule matches.
				return DisplayACLAction("reject") + " (default)"
			}

			return DisplayACLAction(action)
		}

		output += utils.WithPadding("On the network: ", padding) + strings.Join(onNetwork, ", ") + "\n"
		output += utils.WithPadding("Unmatched ingress: ", padding) + unmatched("ingress") + "\n"
		output += utils.WithPadding("Unmatched egress: ", padding) + unmatched("egress") + "\n"
	}

	nics := lo.Keys(onNICs)
	slices.Sort(nics)

	if len(nics) > 0 {
		table, err := utils.RenderTable(lo.Map(nics, func(nic string, _ int) []string {
			return []string{"  " + nic, strings.Join(onNICs[nic], ", ")}
		}))
		if err != nil {
			return err.Error()
		}

		output += "\nOn single NICs:\n" + table + "\n"
	}

	names := lo.Uniq(append(slices.Clone(onNetwork), lo.Flatten(lo.Values(onNICs))...))
	slices.Sort(names)

	for _, name := range names {
		output += "\n" + SectionHeading(name, width) + "\n"

		acl, err := network.ACL(name)
		if err != nil {
			output += err.Error() + "\n"
			continue
		}

		if acl.Description != "" {
			output += acl.Description + "\n"
		}

		for _, direction := range []struct {
			title string
			rules []api.NetworkACLRule
		}{{tr.ACLIngress, acl.Ingress}, {tr.ACLEgress, acl.Egress}} {
			if len(direction.rules) == 0 {
				continue
			}

			table, err := utils.RenderTable(GetACLRuleRows(direction.rules))
			if err != nil {
				return err.Error()
			}

			output += "\n" + direction.title + ":\n" + table + "\n"
		}
	}

	return output
}

func NetworkForwards(network *commands.Network, instances []*commands.Instance, tr *i18n.TranslationSet) string {
	if !network.IsManaged() {
		return tr.NoForwardsUnmanaged
	}

	forwards, err := network.Forwards()
	if err != nil {
		return tr.CannotListForwards + "\n\n" + err.Error()
	}

	if len(forwards) == 0 {
		return tr.NoForwards
	}

	owners := addressOwners(instances)

	table, err := utils.RenderTable(GetNetworkForwardRows(forwards, func(address string) string {
		return owners[address]
	}))
	if err != nil {
		return err.Error()
	}

	return table
}

// addressOwners names the instance holding each address, from the newest
// instance listing.
func addressOwners(instances []*commands.Instance) map[string]string {
	owners := map[string]string{}

	for _, instance := range instances {
		for _, family := range []string{"inet", "inet6"} {
			for _, address := range instance.Latest().Addresses(family) {
				owners[address] = instance.Name
			}
		}
	}

	return owners
}

func NetworkConfig(network *commands.Network, loc Location) string {
	output := LocationLines(loc)
	output += utils.WithPadding("Name: ", IdentityPadding) + network.Name + "\n"
	output += utils.WithPadding("Type: ", IdentityPadding) + network.Network.Type + "\n"
	output += utils.WithPadding("Managed: ", IdentityPadding) + fmt.Sprint(network.IsManaged()) + "\n"
	output += utils.WithPadding("Used by: ", IdentityPadding) + fmt.Sprint(network.UsedByCount()) + "\n"

	data, err := utils.MarshalIntoYaml(network.Network)
	if err != nil {
		return fmt.Sprintf("Error marshalling network details: %v", err)
	}

	return output + fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))
}
