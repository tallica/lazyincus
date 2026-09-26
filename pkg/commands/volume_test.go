package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func TestVolumeUsers(t *testing.T) {
	volume := &Volume{Volume: api.StorageVolume{Project: "default", Type: "custom", UsedBy: []string{
		"/1.0/instances/web",
		"/1.0/instances/db-1?project=stack",
		"/1.0/profiles/shared%20data",
	}}}

	assert.Equal(t, []string{"web", "db-1 (stack)", "profile shared data"}, volume.Users())
	assert.False(t, volume.IsOrphaned())

	assert.True(t, (&Volume{Volume: api.StorageVolume{Type: "custom"}}).IsOrphaned())
	assert.False(t, (&Volume{Volume: api.StorageVolume{Type: "image"}}).IsOrphaned())
}
