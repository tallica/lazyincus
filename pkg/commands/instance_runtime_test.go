package commands

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func listed(project, name, status string) *Instance {
	return &Instance{
		Name:     name,
		Project:  project,
		Instance: api.InstanceFull{Instance: api.Instance{Name: name, Project: project, Status: status}},
	}
}

func TestLatestFollowsTheNextRefresh(t *testing.T) {
	var runtimes instanceRuntimes

	first := listed("default", "web", "Running")
	runtimes.attach(first, 1)

	second := listed("default", "web", "Stopped")
	runtimes.attach(second, 2)

	assert.NotSame(t, first, second)
	assert.Same(t, second, first.Latest())
	assert.Equal(t, "Stopped", first.Latest().Instance.Status)
}

func TestRuntimeStateSurvivesARefresh(t *testing.T) {
	var runtimes instanceRuntimes

	first := listed("default", "web", "Running")
	runtimes.attach(first, 1)
	first.runtime.appendToLog([]byte("booted\n"))
	first.setTopCommand([]string{"ps"})

	second := listed("default", "web", "Running")
	runtimes.attach(second, 2)

	assert.Equal(t, "booted\n", second.runtime.logBuffer.String())
	assert.Equal(t, [][]string{{"ps"}}, second.candidateTopCommands()[:1])
}

func TestSameNameInAnotherProjectIsAnotherInstance(t *testing.T) {
	var runtimes instanceRuntimes

	alpha := listed("alpha", "web", "Running")
	beta := listed("beta", "web", "Stopped")
	runtimes.attach(alpha, 1)
	runtimes.attach(beta, 1)

	assert.Same(t, alpha, alpha.Latest())
	assert.Same(t, beta, beta.Latest())
}

func TestPruneOnlyCoversTheListedProjects(t *testing.T) {
	var runtimes instanceRuntimes

	gone := listed("alpha", "old", "Stopped")
	kept := listed("alpha", "web", "Running")
	elsewhere := listed("beta", "db", "Running")

	for _, instance := range []*Instance{gone, kept, elsewhere} {
		runtimes.attach(instance, 1)
	}

	runtimes.prune(func(project string) bool { return project == "alpha" }, []*Instance{kept}, 2)

	assert.NotContains(t, runtimes.byKey, gone.Key())
	assert.Contains(t, runtimes.byKey, kept.Key())
	assert.Contains(t, runtimes.byKey, elsewhere.Key())
}

// The poll and an action's own refresh overlap, and the older of the two
// can answer last.
func TestALateListingLeavesTheNewerLatest(t *testing.T) {
	var runtimes instanceRuntimes

	older, newer := runtimes.beginListing(), runtimes.beginListing()

	stopped := listed("default", "web", "Stopped")
	runtimes.attach(stopped, newer)

	running := listed("default", "web", "Running")
	runtimes.attach(running, older)

	assert.Same(t, stopped, running.Latest())
}

func TestALateListingDoesNotPruneWhatANewerOneSaw(t *testing.T) {
	var runtimes instanceRuntimes

	older, newer := runtimes.beginListing(), runtimes.beginListing()

	created := listed("alpha", "web", "Running")
	runtimes.attach(created, newer)

	runtimes.prune(func(string) bool { return true }, nil, older)

	assert.Contains(t, runtimes.byKey, created.Key())
}

func TestSnapshotsComeFromTheListing(t *testing.T) {
	instance := listed("alpha", "web", "Running")
	instance.Instance.Snapshots = []api.InstanceSnapshot{{Name: "web/daily"}, {Name: "initial"}}

	snapshots := instance.Snapshots()

	assert.Len(t, snapshots, 2)
	assert.Equal(t, "daily", snapshots[0].Name)
	assert.Equal(t, "alpha/web/daily", snapshots[0].Key())
	assert.Equal(t, "initial", snapshots[1].Name)
}

func TestKeysIncludeTheProject(t *testing.T) {
	assert.NotEqual(t, listed("alpha", "web", "").Key(), listed("beta", "web", "").Key())

	network := func(project string) *Network {
		return &Network{Name: "br0", Network: api.Network{Project: project}}
	}
	assert.NotEqual(t, network("alpha").Key(), network("beta").Key())

	image := func(project string) *Image {
		return &Image{Fingerprint: "abc", Image: api.Image{Project: project}}
	}
	assert.NotEqual(t, image("alpha").Key(), image("beta").Key())

	snapshot := func(project string) *Snapshot {
		return &Snapshot{Project: project, Owner: "web", Name: "daily"}
	}
	assert.NotEqual(t, snapshot("alpha").Key(), snapshot("beta").Key())
}

func TestVolumeSnapshots(t *testing.T) {
	expires := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	volume := &Volume{
		Pool: "default", Name: "data",
		Volume: api.StorageVolume{Type: "custom", Project: "stack"},
		SnapshotList: []api.StorageVolumeSnapshot{{
			Name: "data/nightly", CreatedAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
			StorageVolumeSnapshotPut: api.StorageVolumeSnapshotPut{ExpiresAt: &expires},
		}},
	}

	snapshots := volume.Snapshots()
	require.Len(t, snapshots, 1)

	snapshot := snapshots[0]
	assert.Equal(t, "nightly", snapshot.Name)
	assert.Equal(t, "data", snapshot.Owner)
	assert.Equal(t, expires, snapshot.ExpiresAt())
	assert.False(t, snapshot.IsStateful())

	// Not the key of an instance snapshot of the same names.
	instanceSnapshot := &Snapshot{Project: "stack", Owner: "data", Name: "nightly"}
	assert.NotEqual(t, instanceSnapshot.Key(), snapshot.Key())
}

func TestTransitionOutlivesARefresh(t *testing.T) {
	var runtimes instanceRuntimes

	first := listed("default", "vm", "Running")
	runtimes.attach(first, 1)

	end := first.BeginTransition("Restarting")

	second := listed("default", "vm", "Running")
	runtimes.attach(second, 2)
	assert.Equal(t, "Restarting", second.Status())

	end()
	assert.Equal(t, "Running", second.Status())
}

// consoleInstance is an instance whose console log the stand-in serves.
func consoleInstance(kind, status, log string) *Instance {
	instance := listed("default", "web", status)
	instance.Instance.Type = kind
	instance.Client = incustest.New(incustest.Server{ConsoleLogs: map[string]string{"web": log}})

	return instance
}

// A VM's console log comes back whole each time: the daemon keeps it in a
// file. Read three times, it's still there once.
func TestAVMsConsoleLogIsNotRepeated(t *testing.T) {
	instance := consoleInstance("virtual-machine", "Running", "Booting Debian\n")

	for range 3 {
		_, err := instance.TailConsoleLog()
		require.NoError(t, err)
	}

	log, err := instance.TailConsoleLog()
	require.NoError(t, err)
	assert.Equal(t, "Booting Debian\n", log)
}

// A running container's read is only what arrived since the last one.
func TestARunningContainersConsoleLogAccumulates(t *testing.T) {
	instance := consoleInstance("container", "Running", "line\n")

	_, err := instance.TailConsoleLog()
	require.NoError(t, err)

	log, err := instance.TailConsoleLog()
	require.NoError(t, err)
	assert.Equal(t, "line\nline\n", log)
}

// Stopped, any instance's read is the whole log; it replaces what the
// running reads gathered rather than repeating it.
func TestAStoppedContainersWholeLogReplacesTheBuffer(t *testing.T) {
	instance := consoleInstance("container", "Running", "started\n")

	_, err := instance.TailConsoleLog()
	require.NoError(t, err)

	instance.Instance.Status = "Stopped"
	instance.Client = incustest.New(incustest.Server{ConsoleLogs: map[string]string{"web": "started\nstopped\n"}})

	log, err := instance.TailConsoleLog()
	require.NoError(t, err)
	assert.Equal(t, "started\nstopped\n", log)
}

// An action and the event for it both mark the instance; the one that
// ends first mustn't take the other's mark with it.
func TestOnlyTheLatestMarkEndsATransition(t *testing.T) {
	instance := listed("default", "vm", "Running")

	endAction := instance.BeginTransition("Restarting")
	endEvent := instance.BeginTransition("Restarting")

	endAction()
	assert.Equal(t, "Restarting", instance.Status())

	endEvent()
	assert.Equal(t, "Running", instance.Status())
}

func TestMarkInstanceFindsAListedInstance(t *testing.T) {
	var command IncusCommand

	instance := listed("default", "vm", "Running")
	command.runtimes.attach(instance, 1)

	end := command.MarkInstance("default", "vm", "Stopping")
	assert.Equal(t, "Stopping", instance.Status())

	end()
	assert.Equal(t, "Running", instance.Status())

	command.MarkInstance("default", "unlisted", "Stopping")()
}

// ic-healthd rewrites its verdict every few seconds; a tab redrawn on each
// would never hold still. Any other change to the config counts.
func TestConfigFingerprintIgnoresHealthVerdicts(t *testing.T) {
	instance := listed("default", "web", "Running")
	instance.Instance.Config = map[string]string{"user.healthcheck.status": "healthy", "limits.cpu": "2"}
	before := instance.ConfigFingerprint()

	instance.Instance.Config = map[string]string{"user.healthcheck.status": "unhealthy", "limits.cpu": "2"}
	assert.Equal(t, before, instance.ConfigFingerprint())

	instance.Instance.Config = map[string]string{"user.healthcheck.status": "unhealthy", "limits.cpu": "4"}
	assert.NotEqual(t, before, instance.ConfigFingerprint())
}
