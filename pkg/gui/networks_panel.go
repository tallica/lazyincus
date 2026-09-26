package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getNetworksPanel() *panels.SideListPanel[*commands.Network] {
	return &panels.SideListPanel[*commands.Network]{
		ContextState: &panels.ContextState[*commands.Network]{
			GetMainTabs: func() []panels.MainTab[*commands.Network] {
				return []panels.MainTab[*commands.Network]{
					{
						Key:    "leases",
						Title:  gui.Tr.LeasesTitle,
						Render: gui.renderNetworkLeases,
					},
					{
						Key:    "state",
						Title:  gui.Tr.StateTitle,
						Render: gui.renderNetworkState,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderNetworkConfig,
					},
				}
			},
			GetItemContextCacheKey: func(network *commands.Network) string {
				return "networks-" + network.Key()
			},
		},
		ListPanel: panels.ListPanel[*commands.Network]{
			List: panels.NewFilteredList[*commands.Network](),
			View: gui.Views.Networks,
		},
		NoItemsMessage: gui.Tr.NoNetworks,
		Gui:            gui.intoInterface(),
		// The host's own interfaces outnumber Incus's networks on most
		// machines, and there's nothing to do with them from here.
		Filter: func(network *commands.Network) bool {
			return gui.State.ShowUnmanagedNetworks || network.IsManaged()
		},
		Sort: func(a *commands.Network, b *commands.Network) bool {
			return a.Name < b.Name
		},
		SameItem: func(a, b *commands.Network) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(item *commands.Network) []string {
			return presentation.GetNetworkDisplayStrings(item, gui.State.SpansProjects.Networks)
		},
	}
}

func (gui *Gui) handleToggleUnmanagedNetworks(g *gocui.Gui, v *gocui.View) error {
	gui.State.ShowUnmanagedNetworks = !gui.State.ShowUnmanagedNetworks

	return gui.Panels.Networks.RerenderList()
}

// renderNetworkLeases polls the leases: an instance starting takes one
// without anything the panel's own refresh would notice.
func (gui *Gui) renderNetworkLeases(network *commands.Network) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.networkLeasesStr(network))
		},
		Duration: time.Second * 5,
		Before:   gui.clearMain,
	})
}

func (gui *Gui) networkLeasesStr(network *commands.Network) string {
	if !network.IsManaged() {
		return gui.Tr.NoLeasesUnmanaged
	}

	leases, err := network.Leases()
	if err != nil {
		return gui.Tr.CannotListLeases + "\n\n" + err.Error()
	}

	if len(leases) == 0 {
		return gui.Tr.NoLeases
	}

	table, err := utils.RenderTable(presentation.GetNetworkLeaseRows(leases))
	if err != nil {
		return err.Error()
	}

	return table
}

func (gui *Gui) renderNetworkState(network *commands.Network) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.networkStateStr(network))
		},
		Duration: time.Second * 2,
		Before:   gui.clearMain,
	})
}

// networkStateStr is `incus network info`, with a bridge's ports named by
// the instance on the other end rather than by veth alone.
func (gui *Gui) networkStateStr(network *commands.Network) string {
	state, err := network.State()
	if err != nil {
		return gui.Tr.CannotReadNetworkState + "\n\n" + err.Error()
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

			owners := gui.hostInterfaceOwners()
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
func (gui *Gui) hostInterfaceOwners() map[string]string {
	owners := map[string]string{}

	for _, instance := range gui.Panels.Instances.List.GetAllItems() {
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

func (gui *Gui) renderNetworkConfig(network *commands.Network) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.networkConfigStr(network) })
}

func (gui *Gui) networkConfigStr(network *commands.Network) string {
	padding := 12
	output := ""
	output += utils.WithPadding("Name: ", padding) + network.Name + "\n"
	output += utils.WithPadding("Type: ", padding) + network.Network.Type + "\n"
	output += utils.WithPadding("Managed: ", padding) + fmt.Sprint(network.IsManaged()) + "\n"
	output += utils.WithPadding("Used by: ", padding) + fmt.Sprint(network.UsedByCount()) + "\n"

	data, err := utils.MarshalIntoYaml(network.Network)
	if err != nil {
		return fmt.Sprintf("Error marshalling network details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) fetchNetworks() (func() error, error) {
	ticket := gui.refreshes.networks.issue()

	networks, err := gui.IncusCommand.GetNetworks()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.networks.admit(ticket) {
			return nil
		}

		gui.State.SpansProjects.Networks = spansMultipleProjects(
			lo.Map(networks, func(network *commands.Network, _ int) string { return network.Network.Project }))

		gui.Panels.Networks.SetItems(networks)

		return gui.Panels.Networks.RerenderList()
	}, nil
}

func (gui *Gui) refreshNetworks() error {
	return gui.refresh(nil, gui.fetchNetworks)
}

func (gui *Gui) refreshNetworksQuiet() error {
	if err := gui.refreshNetworks(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

func (gui *Gui) showNetworkUsers(network *commands.Network) error {
	return gui.showUsers(network.Name, network.IsUsedBy)
}

func (gui *Gui) networkEdit(network *commands.Network) error {
	if !network.IsManaged() {
		return gui.createErrorPanel(gui.Tr.CannotEditUnmanagedNetwork)
	}

	return gui.editInIncus(network.Network.Project, gui.fetchNetworks, "network", "edit", network.Name)
}

func (gui *Gui) networkDelete(network *commands.Network) error {
	if !network.IsManaged() {
		return gui.createErrorPanel(gui.Tr.CannotDeleteUnmanagedNetwork)
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteNetwork, network.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := network.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshNetworks()
		})
	}, nil)
}
