package gui

import (
	"fmt"

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
