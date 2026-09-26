package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func instanceWith(project, name string, devices map[string]map[string]string) *Instance {
	return &Instance{Name: name, Project: project, Instance: api.InstanceFull{Instance: api.Instance{
		Name: name, Project: project, ExpandedDevices: devices,
	}}}
}

func TestNetworkIsUsedBy(t *testing.T) {
	// used_by names the profile, not the instances that have it.
	network := &Network{Name: "incusbr0", Network: api.Network{
		Project: "default", UsedBy: []string{"/1.0/profiles/default", "/1.0/instances/named"},
	}}

	viaProfile := instanceWith("stack", "web-1", map[string]map[string]string{
		"eth0": {"type": "nic", "network": "incusbr0"},
	})
	bridged := instanceWith("default", "old", map[string]map[string]string{
		"eth0": {"type": "nic", "nictype": "bridged", "parent": "incusbr0"},
	})
	elsewhere := instanceWith("default", "db", map[string]map[string]string{
		"eth0": {"type": "nic", "network": "other"},
	})

	assert.True(t, network.IsUsedBy(viaProfile))
	assert.True(t, network.IsUsedBy(bridged))
	assert.True(t, network.IsUsedBy(instanceWith("default", "named", nil)), "used_by without a project is default's")
	assert.False(t, network.IsUsedBy(elsewhere))

	// A project's own network is out of reach of another project's instances.
	own := &Network{Name: "incusbr0", Network: api.Network{Project: "tenant"}}
	assert.False(t, own.IsUsedBy(viaProfile))
}

func TestVolumeIsUsedBy(t *testing.T) {
	data := &Volume{Pool: "default", Name: "vol-redis-data", Volume: api.StorageVolume{Type: "custom", Project: "stack"}}

	redis := instanceWith("stack", "redis-1", map[string]map[string]string{
		"data": {"type": "disk", "pool": "default", "source": "vol-redis-data", "path": "/data"},
	})

	assert.True(t, data.IsUsedBy(redis))
	assert.False(t, data.IsUsedBy(instanceWith("stack", "web-1", nil)))

	root := &Volume{Pool: "default", Name: "web-1", Volume: api.StorageVolume{
		Type: "container", Project: "stack", UsedBy: []string{"/1.0/instances/web-1?project=stack"},
	}}
	assert.True(t, root.IsUsedBy(instanceWith("stack", "web-1", nil)))
	assert.False(t, root.IsUsedBy(instanceWith("other", "web-1", nil)))
}
