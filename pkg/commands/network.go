package commands

import (
	"net/url"
	"slices"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Network is a network as listed by `incus network list`, managed by Incus
// or merely detected on the host.
type Network struct {
	Name string

	Network api.Network
	Client  incus.InstanceServer
	// ClientFor is a client scoped to another project, which the leases
	// need: see Leases.
	ClientFor func(project string) incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

// Key identifies the network across refreshes: the all-projects view can
// list the same name once per project.
func (n *Network) Key() string {
	return n.Network.Project + "/" + n.Name
}

func (n *Network) IsManaged() bool {
	return n.Network.Managed
}

// UsedByCount is how many profiles and instances reference the network.
func (n *Network) UsedByCount() int {
	return len(n.Network.UsedBy)
}

// Leases are what the network's DHCP server has handed out, plus the
// addresses it keeps for itself. Only managed networks have any. The daemon
// answers with the asking project's leases alone, so this asks every
// project the network's used_by names too - see docs/Incus.md.
func (n *Network) Leases() ([]api.NetworkLease, error) {
	leases, err := n.Client.GetNetworkLeases(n.Name)
	if err != nil {
		return nil, err
	}

	if n.ClientFor == nil {
		return leases, nil
	}

	for _, project := range n.usedByProjects() {
		more, err := n.ClientFor(project).GetNetworkLeases(n.Name)
		if err != nil {
			// A project that has gone since the listing; the rest still
			// stand.
			continue
		}

		for _, lease := range more {
			if !slices.Contains(leases, lease) {
				leases = append(leases, lease)
			}
		}
	}

	return leases, nil
}

// usedByProjects are the projects other than the network's own that its
// used_by URLs name.
func (n *Network) usedByProjects() []string {
	own := n.Network.Project
	if own == "" {
		own = api.ProjectDefaultName
	}

	projects := []string{}

	for _, entry := range n.Network.UsedBy {
		parsed, err := url.Parse(entry)
		if err != nil {
			continue
		}

		project := parsed.Query().Get("project")
		if project == "" {
			project = api.ProjectDefaultName
		}

		if project != own && !slices.Contains(projects, project) {
			projects = append(projects, project)
		}
	}

	return projects
}

// State is the interface as the host sees it: addresses, counters and, for
// a bridge, the ports on it.
func (n *Network) State() (*api.NetworkState, error) {
	return n.Client.GetNetworkState(n.Name)
}

// Delete removes the network. Incus only allows this for managed networks
// that nothing is using; it rejects the rest itself.
func (n *Network) Delete() error {
	return n.Client.DeleteNetwork(n.Name)
}
