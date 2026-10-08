package commands

import (
	"net/url"
	"slices"
	"strings"
	"sync"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/rs/zerolog"
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
	Log       *zerolog.Logger
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

	projects := n.usedByProjects()
	answers := make([][]api.NetworkLease, len(projects))

	var wait sync.WaitGroup

	slots := make(chan struct{}, requestsInFlight)

	for i, project := range projects {
		wait.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			// An error is a project gone since the listing; the rest stand.
			if more, err := n.ClientFor(project).GetNetworkLeases(n.Name); err == nil {
				answers[i] = more
			}
		})
	}

	wait.Wait()

	// Merged in used_by's order, not the answers', so rows hold still from
	// one refresh to the next.
	for _, more := range answers {
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

// ACLNames are the ACLs the network applies to everything on it.
func (n *Network) ACLNames() []string {
	return splitACLs(n.Network.Config["security.acls"])
}

// NICACLs names each NIC on the network with ACLs of its own, as
// "instance (nic)", against those ACLs.
func (n *Network) NICACLs(instances []*Instance) map[string][]string {
	nics := map[string][]string{}

	for _, instance := range instances {
		for device, config := range instance.Instance.ExpandedDevices {
			if config["type"] != "nic" || (config["network"] != n.Name && config["parent"] != n.Name) {
				continue
			}

			if acls := splitACLs(config["security.acls"]); len(acls) > 0 && reachable(n.Network.Project, instance) {
				nics[instance.Name+" ("+device+")"] = acls
			}
		}
	}

	return nics
}

func (n *Network) ACL(name string) (*api.NetworkACL, error) {
	acl, _, err := n.Client.GetNetworkACL(name)

	return acl, err
}

func splitACLs(value string) []string {
	acls := []string{}

	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			acls = append(acls, name)
		}
	}

	return acls
}

// Forwards are the network's forwards: a listen address on the uplink,
// each port on it sent to an address inside.
func (n *Network) Forwards() ([]api.NetworkForward, error) {
	return n.Client.GetNetworkForwards(n.Name)
}

// Delete removes the network. Incus only allows this for managed networks
// that nothing is using; it rejects the rest itself.
func (n *Network) Delete() error {
	return n.Client.DeleteNetwork(n.Name)
}
