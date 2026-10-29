package gui

import (
	"context"
	"fmt"
	"strconv"
	"time"

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
						Key:    "acls",
						Title:  gui.Tr.ACLsTitle,
						Render: gui.renderNetworkACLs,
					},
					{
						Key:    "forwards",
						Title:  gui.Tr.ForwardsTitle,
						Render: gui.renderNetworkForwards,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderNetworkConfig,
					},
				}
			},
			GetItemContextCacheKey: func(network *commands.Network) string {
				// Forwards and ACLs aren't in the network; their events count
				// towards networkTabs instead.
				return "networks-" + network.Key() + "-" + utils.Fingerprint(network.Network) +
					"-" + strconv.FormatUint(gui.networkTabs.Load(), 10)
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
	return presentation.NetworkLeases(network, gui.Tr)
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

func (gui *Gui) networkStateStr(network *commands.Network) string {
	return presentation.NetworkState(network, gui.Panels.Instances.List.GetAllItems(), gui.Tr)
}

func (gui *Gui) renderNetworkACLs(network *commands.Network) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.networkACLsStr(network) })
}

func (gui *Gui) networkACLsStr(network *commands.Network) string {
	return presentation.NetworkACLs(network, gui.Panels.Instances.List.GetAllItems(), int(gui.mainViewWidth.Load()), gui.Tr)
}

func (gui *Gui) renderNetworkForwards(network *commands.Network) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.networkForwardsStr(network) })
}

func (gui *Gui) networkForwardsStr(network *commands.Network) string {
	return presentation.NetworkForwards(network, gui.Panels.Instances.List.GetAllItems(), gui.Tr)
}

func (gui *Gui) renderNetworkConfig(network *commands.Network) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.networkConfigStr(network) })
}

func (gui *Gui) networkConfigStr(network *commands.Network) string {
	return presentation.NetworkConfig(network, gui.sessionLocation(network.Network.Project))
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
		gui.Log.Warn().Err(err).Send()
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

	return gui.editInIncus(network.Network.Project, []fetch{gui.fetchNetworks}, "network", "edit", network.Name)
}

func (gui *Gui) networkDelete(network *commands.Network) error {
	if !network.IsManaged() {
		return gui.createErrorPanel(gui.Tr.CannotDeleteUnmanagedNetwork)
	}

	name := gui.qualified(network.Name, network.Network.Project, gui.State.SpansProjects.Networks)
	if network.UsedByCount() > 0 {
		return gui.refuseInUse(fmt.Sprintf(gui.Tr.NetworkNamed, name), network.UsedByCount())
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteNetwork, name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := network.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshNetworks()
		})
	}, nil)
}
