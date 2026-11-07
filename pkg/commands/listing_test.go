package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func listingServer() *incustest.Server {
	instance := func(project, name string) api.InstanceFull {
		return api.InstanceFull{Instance: api.Instance{
			Name: name, Project: project,
			ExpandedConfig: map[string]string{composeServiceKey: name},
			InstancePut:    api.InstancePut{Config: map[string]string{baseImageKey: "used"}},
		}}
	}

	return incustest.New(incustest.Server{
		Instances: []api.InstanceFull{instance("default", "app"), instance("web", "web-1"), instance("web", "db-1")},
		Images:    []api.Image{{Fingerprint: "used", Project: "default"}},
	})
}

// readRound is what a full refresh reads: every one of them lists instances.
func readRound(t *testing.T, command *IncusCommand, l *Listing) {
	t.Helper()

	instances, err := command.GetInstances(l)
	require.NoError(t, err)
	assert.NotEmpty(t, instances)

	images, err := command.GetImages(l)
	require.NoError(t, err)
	assert.Equal(t, []string{"default/app", "web/web-1", "web/db-1"}, images[0].UsedBy)

	statuses, err := command.GetComposeStatuses(l)
	require.NoError(t, err)
	assert.Len(t, statuses, 2)

	inWeb, err := command.GetProjectInstances("web", l)
	require.NoError(t, err)
	assert.Len(t, inWeb, 2)
}

func TestListingAllProjectsListsOnce(t *testing.T) {
	server := listingServer()
	command := NewIncusCommandWithClient(NewDummyLog(), NewDummyOSCommand(), NewDummyAppConfig(), server, "fake")
	command.UseAllProjects()

	l := command.NewListing()
	l.WithScoped()
	readRound(t, command, l)
	assert.Equal(t, 1, server.InstanceListings())

	readRound(t, command, nil)
	assert.Equal(t, 1+4, server.InstanceListings(), "without a listing, each reads its own")
}

// Without the panel's listing, the others aren't taken from it: they share
// the lighter all-projects listing instead.
func TestListingWithoutScopedSharesTheLighterListing(t *testing.T) {
	server := listingServer()
	command := NewIncusCommandWithClient(NewDummyLog(), NewDummyOSCommand(), NewDummyAppConfig(), server, "fake")
	command.UseAllProjects()

	l := command.NewListing()
	images, err := command.GetImages(l)
	require.NoError(t, err)
	assert.Len(t, images[0].UsedBy, 3)

	statuses, err := command.GetComposeStatuses(l)
	require.NoError(t, err)
	assert.Len(t, statuses, 2)

	inWeb, err := command.GetProjectInstances("web", l)
	require.NoError(t, err)
	assert.Len(t, inWeb, 2)

	assert.Equal(t, 2, server.InstanceListings(), "every project's once, the stack's project once")
}

func TestListingOneProjectSharesTheAllProjectsListing(t *testing.T) {
	server := listingServer()
	command := NewIncusCommandWithClient(NewDummyLog(), NewDummyOSCommand(), NewDummyAppConfig(), server, "fake")
	command.UseProject("default")

	l := command.NewListing()
	l.WithScoped()
	readRound(t, command, l)
	assert.Equal(t, 3, server.InstanceListings(), "the panel's project, every project, and the stack's")

	before := server.InstanceListings()
	l = command.NewListing()
	_, err := command.GetInstances(l)
	require.NoError(t, err)
	_, err = command.GetProjectInstances("default", l)
	require.NoError(t, err)
	assert.Equal(t, before+1, server.InstanceListings(), "a stack in the panel's project is the panel's listing")
}

// Another remote's command doesn't take this one's listing.
func TestListingStaysWithItsRemote(t *testing.T) {
	here, there := listingServer(), listingServer()
	command := NewIncusCommandWithClient(NewDummyLog(), NewDummyOSCommand(), NewDummyAppConfig(), here, "fake")
	other := NewIncusCommandWithClient(NewDummyLog(), NewDummyOSCommand(), NewDummyAppConfig(), there, "fake")

	l := command.NewListing()
	_, err := command.GetComposeStatuses(l)
	require.NoError(t, err)
	_, err = other.GetComposeStatuses(l)
	require.NoError(t, err)

	assert.Equal(t, 1, here.InstanceListings())
	assert.Equal(t, 1, there.InstanceListings())
}
