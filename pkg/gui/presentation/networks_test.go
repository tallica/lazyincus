package presentation

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/utils"
)

func TestGetNetworkLeaseRows(t *testing.T) {
	rows := GetNetworkLeaseRows([]api.NetworkLease{
		{Hostname: "web", Hwaddr: "10:66:6a:00:00:02", Address: "10.0.0.20", Type: "DYNAMIC"},
		{Hostname: "db", Hwaddr: "10:66:6a:00:00:01", Address: "fd42::1", Type: "STATIC"},
		{Hostname: "incusbr0.gw", Address: "10.0.0.1", Type: "GATEWAY"},
		{Hostname: "web", Hwaddr: "10:66:6a:00:00:02", Address: "fd42::20", Type: "DYNAMIC"},
		{Hostname: "db", Hwaddr: "10:66:6a:00:00:01", Address: "10.0.0.10", Type: "STATIC"},
	})

	plain := make([][]string, len(rows))
	for i, row := range rows {
		plain[i] = make([]string, len(row))
		for j, cell := range row {
			plain[i][j] = utils.Decolorise(cell)
		}
	}

	// One row a host, both families on it, the gateway ahead of the rest.
	assert.Equal(t, [][]string{
		{"HOSTNAME", "IPV4", "IPV6", "MAC", "TYPE"},
		{"incusbr0.gw", "10.0.0.1", "", "", "gateway"},
		{"db", "10.0.0.10", "fd42::1", "10:66:6a:00:00:01", "static"},
		{"web", "10.0.0.20", "fd42::20", "10:66:6a:00:00:02", "dynamic"},
	}, plain)
}
