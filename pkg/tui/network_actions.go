package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// onNetwork is pkg/gui's onSelected for the networks panel.
func onNetwork(action func(*Model, *commands.Network) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		network := m.selectedNetwork()
		if network == nil {
			return nil
		}

		return action(m, network)
	}
}

func (m *Model) networkEdit(network *commands.Network) tea.Cmd {
	if !network.IsManaged() {
		m.openError(m.tr.CannotEditUnmanagedNetwork)
		return nil
	}

	return m.editInIncus(network.Network.Project, func() []fetch { return []fetch{m.fetchNetworks()} }, "network", "edit", network.Name)
}

func (m *Model) networkUsers(network *commands.Network) tea.Cmd {
	return m.showUsers(network.Name, network.IsUsedBy)
}

func (m *Model) networkCopy(network *commands.Network) tea.Cmd {
	m.copyMenu(
		copyValue{m.tr.CopyName, network.Name},
		// The gateway with its prefix, as Incus's ipv4.address has it; not the subnet's own address.
		copyValue{"IPv4 address", network.Network.Config["ipv4.address"]},
		copyValue{"IPv6 address", network.Network.Config["ipv6.address"]},
	)

	return nil
}

// networkDelete is pkg/gui's: a managed network, and one nothing uses.
func (m *Model) networkDelete(network *commands.Network) tea.Cmd {
	if !network.IsManaged() {
		m.openError(m.tr.CannotDeleteUnmanagedNetwork)
		return nil
	}

	name := m.qualified(network.Name, network.Network.Project, m.networksSpan)
	if network.UsedByCount() > 0 {
		m.refuseInUse(fmt.Sprintf(m.tr.NetworkNamed, name), network.UsedByCount())
		return nil
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.DeleteNetwork, name), func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, network.Delete, m.reportOr(func() tea.Cmd {
			return refresh(m.fetchNetworks())
		}))
	})

	return nil
}
