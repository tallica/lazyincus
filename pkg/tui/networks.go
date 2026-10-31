package tui

import (
	"sort"
	"strconv"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) fetchNetworks() fetch {
	ticket := m.refreshes.networks.issue()

	return func() (func(), error) {
		networks, err := m.incus.GetNetworks()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.networks.admit(ticket) {
				m.setNetworks(networks)
			}
		}, nil
	}
}

func (m *Model) setNetworks(networks []*commands.Network) {
	m.refill(m.panelNamed("networks"), func(item int) string { return m.networks[item].Key() }, func() [][]string {
		sort.SliceStable(networks, func(a, b int) bool { return networks[a].Name < networks[b].Name })
		m.networks = networks
		m.networksSpan = spansProjects(networks, networkProject)

		cells := make([][]string, len(networks))
		for i, network := range networks {
			cells[i] = presentation.GetNetworkDisplayStrings(network, m.networksSpan)
		}

		return cells
	})
}

func networkProject(network *commands.Network) string { return network.Network.Project }

// keepNetwork: the host's own interfaces wait for e.
func (m *Model) keepNetwork(item int) bool {
	return m.showUnmanaged || m.networks[item].IsManaged()
}

func (m *Model) toggleUnmanagedNetworks() {
	m.showUnmanaged = !m.showUnmanaged
	m.filterList(m.panelNamed("networks"))
}

func (m *Model) selectedNetwork() *commands.Network {
	item := m.panelNamed("networks").item()
	if item < 0 || item >= len(m.networks) {
		return nil
	}

	return m.networks[item]
}

// networkMainTabs poll leases and state, as nothing the
// list notices says an instance took a lease or a port changed.
func (m *Model) networkMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "leases", title: m.tr.LeasesTitle, task: m.networkTicker(5*time.Second, m.networkLeases), wrap: wrap},
		{key: "state", title: m.tr.StateTitle, task: m.networkTicker(2*time.Second, m.networkState), wrap: wrap},
		{key: "acls", title: m.tr.ACLsTitle, task: m.networkTask(m.networkACLs), wrap: wrap},
		{key: "forwards", title: m.tr.ForwardsTitle, task: m.networkTask(m.networkForwards), wrap: wrap},
		{key: "config", title: m.tr.ConfigTitle, task: m.networkTask(m.networkConfig), wrap: wrap},
	}
}

// networkTab makes, on the loop, what a tab renders off it.
type networkTab func(network *commands.Network) func() string

func (m *Model) networkTask(content networkTab) func() mainTaskFunc {
	return func() mainTaskFunc { return renderString(content(m.selectedNetwork())) }
}

func (m *Model) networkTicker(interval time.Duration, content networkTab) func() mainTaskFunc {
	return func() mainTaskFunc { return tickerTask(interval, content(m.selectedNetwork())) }
}

func (m *Model) networkLeases(network *commands.Network) func() string {
	tr := m.tr
	return func() string { return presentation.NetworkLeases(network, tr) }
}

func (m *Model) networkState(network *commands.Network) func() string {
	instances, tr := m.listedInstances(), m.tr
	return func() string { return presentation.NetworkState(network, instances(), tr) }
}

func (m *Model) networkACLs(network *commands.Network) func() string {
	instances, width, tr := m.listedInstances(), m.main.width, m.tr
	return func() string { return presentation.NetworkACLs(network, instances(), width, tr) }
}

func (m *Model) networkForwards(network *commands.Network) func() string {
	instances, tr := m.listedInstances(), m.tr
	return func() string { return presentation.NetworkForwards(network, instances(), tr) }
}

func (m *Model) listedInstances() func() []*commands.Instance {
	listed := &m.listed

	return func() []*commands.Instance {
		if instances := listed.Load(); instances != nil {
			return *instances
		}

		return nil
	}
}

func (m *Model) networkConfig(network *commands.Network) func() string {
	loc := m.sessionLocation(network.Network.Project)
	return func() string { return presentation.NetworkConfig(network, loc) }
}

// networkKey: forwards and ACLs aren't in the network, so their events count in networkTabs.
func (m *Model) networkKey() string {
	network := m.selectedNetwork()
	if network == nil {
		return ""
	}

	return network.Key() + "-" + utils.Fingerprint(network.Network) + "-" + strconv.FormatUint(m.networkTabs, 10)
}
