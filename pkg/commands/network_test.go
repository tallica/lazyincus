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

	server := incustest.New(incustest.Server{
		NetworkLeases: map[string]map[string][]api.NetworkLease{
			"br": {"default": {gateway}, "stack": {web, db}},
		},
	})

	network := &Network{
		Name: "br",
		Network: api.Network{Name: "br", Project: "default", UsedBy: []string{
			"/1.0/instances/web-1?project=stack",
			"/1.0/instances/db-1?project=stack",
			"/1.0/profiles/default?project=gone",
		}},
		Client:    server.UseProject("default"),
		ClientFor: func(project string) incus.InstanceServer { return server.UseProject(project) },
	}

	leases, err := network.Leases()
	require.NoError(t, err)
	assert.Equal(t, []api.NetworkLease{gateway, web, db}, leases)
}
