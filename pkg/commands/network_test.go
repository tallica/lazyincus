package commands

import (
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// A compose project's instances sit on a network of default's, whose leases
// each project only sees its own of.
func TestNetworkLeasesAskEveryProjectUsingIt(t *testing.T) {
	gateway := api.NetworkLease{Hostname: "br.gw", Address: "10.0.0.1", Type: "GATEWAY"}
	web := api.NetworkLease{Hostname: "web-1", Hwaddr: "10:66:6a:00:00:01", Address: "10.0.0.10", Type: "DYNAMIC"}
	db := api.NetworkLease{Hostname: "db-1", Hwaddr: "10:66:6a:00:00:02", Address: "10.0.0.20", Type: "DYNAMIC"}
	app := api.NetworkLease{Hostname: "app", Hwaddr: "10:66:6a:00:00:03", Address: "10.0.0.30", Type: "DYNAMIC"}

	server := incustest.New(incustest.Server{
		NetworkLeases: map[string]map[string][]api.NetworkLease{
			"br": {"default": {gateway}, "stack": {web, db}, "tenant": {app}},
		},
	})

	network := &Network{
		Name: "br",
		Network: api.Network{Name: "br", Project: "default", UsedBy: []string{
			"/1.0/instances/web-1?project=stack",
			"/1.0/instances/db-1?project=stack",
			"/1.0/instances/app?project=tenant",
			"/1.0/profiles/default?project=gone",
		}},
		Client:    server.UseProject("default"),
		ClientFor: func(project string) incus.InstanceServer { return server.UseProject(project) },
	}

	leases, err := network.Leases()
	require.NoError(t, err)
	// In used_by's order, however the answers arrive.
	assert.Equal(t, []api.NetworkLease{gateway, web, db, app}, leases)
}

func TestNetworkACLs(t *testing.T) {
	network := &Network{Name: "br", Network: api.Network{Project: "default", Config: map[string]string{
		"security.acls": "isolate, web-only ,",
	}}}

	assert.Equal(t, []string{"isolate", "web-only"}, network.ACLNames())

	web := &Instance{Name: "web-1", Project: "stack", Instance: api.InstanceFull{Instance: api.Instance{
		ExpandedDevices: map[string]map[string]string{
			"eth0": {"type": "nic", "network": "br", "security.acls": "web-only"},
			"eth1": {"type": "nic", "network": "other", "security.acls": "elsewhere"},
		},
	}}}
	db := &Instance{Name: "db-1", Project: "stack", Instance: api.InstanceFull{Instance: api.Instance{
		ExpandedDevices: map[string]map[string]string{"eth0": {"type": "nic", "network": "br"}},
	}}}

	assert.Equal(t, map[string][]string{"web-1 (eth0)": {"web-only"}}, network.NICACLs([]*Instance{web, db}))
}
