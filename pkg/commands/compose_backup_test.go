package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `incus-compose backup list --format json`, as 1.3.4 prints it for a stack
// with two volumes: no size key when the pool can't say.
const backupListOutput = `[
  {
    "timestamp": "2026-09-20T15:27:46.833084Z",
    "name": "",
    "volumes": [
      {
        "source": {"project": "playground", "pool": "default", "name": "vol-sqlite-data"},
        "backup": {"project": "playground-backup", "pool": "default", "name": "ic-backup-sqlite-data"}
      },
      {
        "source": {"project": "playground", "pool": "default", "name": "vol-redis-data"},
        "backup": {"project": "playground-backup", "pool": "default", "name": "ic-backup-redis-data"}
      }
    ]
  }
]`

func TestParseComposeBackups(t *testing.T) {
	backups, err := parseComposeBackups([]byte(backupListOutput))
	require.NoError(t, err)
	require.Len(t, backups, 1)

	backup := backups[0]
	assert.Equal(t, "2026-09-20T15:27:46.833084Z", backup.Timestamp)
	assert.Equal(t, 2026, backup.CreatedAt().Year())
	assert.Equal(t, "default", backup.Pool())
	assert.Zero(t, backup.Size)
	assert.Equal(t, "ic-backup-redis-data", backup.Volumes[1].Backup.Name)

	none, err := parseComposeBackups([]byte("[]\n"))
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestIsComposeBackup(t *testing.T) {
	volume := func(project, name string) *Volume {
		return &Volume{Name: name, Volume: api.StorageVolume{Type: "custom", Project: project}}
	}

	for _, name := range []string{"ic-backup-redis-data", "vol-ic-backup-manifest"} {
		backup := volume("playground-backup", name)
		assert.True(t, backup.IsComposeBackup(), name)
		assert.False(t, backup.IsOrphaned(), name)
		assert.Equal(t, "playground", backup.BackupOf())
	}

	assert.False(t, volume("playground", "ic-backup-redis-data").IsComposeBackup())
	assert.False(t, volume("playground-backup", "vol-redis-data").IsComposeBackup())
}

func TestNamedVolumes(t *testing.T) {
	service := ComposeService{Volumes: []string{"redis-data:/data", "./conf:/etc/conf (ro)", "/srv/x:/x", "~/y:/y", "cache:/cache (ro)"}}

	assert.Equal(t, []string{"redis-data", "cache"}, service.NamedVolumes())
}
