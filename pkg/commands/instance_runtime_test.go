package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
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
		return &Snapshot{Project: project, InstanceName: "web", Name: "daily"}
	}
	assert.NotEqual(t, snapshot("alpha").Key(), snapshot("beta").Key())
}
