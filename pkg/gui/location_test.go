package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// identityLines is the lines of an identity block whose label is one of
// labels, in order.
func identityLines(block string, labels ...string) []string {
	var lines []string

	for _, line := range strings.Split(block, "\n") {
		for _, label := range labels {
			if strings.HasPrefix(line, label+":") {
				lines = append(lines, strings.Join(strings.Fields(line), " "))
			}
		}
	}

	return lines
}

var locationLabels = []string{"Remote", "Project", "Stack", "Service"}

// An instance's Info tab says where it lives: its remote and project, and
// for a compose instance where its stack is listed from - or that it
// isn't - and its service.
func TestTheInfoTabSaysWhereAnInstanceLives(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	shop.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, shop)(s)
		s.server.SetInstances([]api.InstanceFull{
			composeFixture("shop", "api-1", "api"),
			composeFixture("shop", "api-2", "api"),
			composeFixture("elsewhere", "job-1", "job"),
			fixtureServer().Instances[0],
		})
	})

	require.Eventually(t, func() bool {
		return len(serviceNames(t, s)) == 3 &&
			len(onLoop(t, s, s.gui.Panels.Instances.List.GetAllItems)) == 4
	}, 5*time.Second, 20*time.Millisecond)

	byName := map[string]*commands.Instance{}
	for _, instance := range onLoop(t, s, s.gui.Panels.Instances.List.GetAllItems) {
		byName[instance.Name] = instance
	}

	info := func(name string) []string {
		return identityLines(s.gui.instanceIdentityStr(byName[name]), locationLabels...)
	}

	assert.Equal(t, []string{
		"Remote: fake", "Project: shop", "Stack: " + shop.Dir, "Service: api",
	}, info("api-2"))
	assert.Equal(t, []string{
		"Remote: fake", "Project: elsewhere", "Stack: not listed", "Service: job",
	}, info("job-1"))
	assert.Equal(t, []string{"Remote: fake", "Project: default"}, info("web"))
}

// A service's Info tab says once where it lives, and its instance blocks
// don't say it again.
func TestServiceInfoSaysWhereItLivesOnce(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	shop.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, shop)(s)
		s.server.SetInstances([]api.InstanceFull{
			composeFixture("shop", "api-1", "api"),
			composeFixture("shop", "api-2", "api"),
		})
	})

	require.Eventually(t, func() bool {
		return len(serviceNames(t, s)) == 3
	}, 5*time.Second, 20*time.Millisecond)

	row := onLoop(t, s, func() *commands.ServiceRow { return s.gui.Panels.Services.List.GetAllItems()[0] })
	info := s.gui.serviceInfoStr(row)

	assert.Equal(t, []string{
		"Remote: fake", "Project: shop", "Stack: " + shop.Dir, "Service: api",
	}, identityLines(info, locationLabels...))
}

// A snapshot names where its instance lives, on another remote too.
func TestASnapshotSaysWhereItsInstanceLives(t *testing.T) {
	instance := composeFixture("shop", "api-1", "api")
	instance.Snapshots = []api.InstanceSnapshot{{Name: "nightly"}}

	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{instance}})
	shop := testStack(t, t.TempDir(), "shop", "api")
	shop.Remote = "pve01"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, shop)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
	})

	require.Eventually(t, func() bool {
		return len(serviceNames(t, s)) == 1
	}, 5*time.Second, 20*time.Millisecond)

	replica := onLoop(t, s, func() *commands.Instance {
		return s.gui.Panels.Services.List.GetAllItems()[0].Service.Instances[0]
	})
	snapshot := &commands.Snapshot{Project: "shop", Owner: "api-1", Name: "nightly", Instance: replica}

	assert.Equal(t, []string{
		"Remote: pve01", "Project: shop", "Stack: " + shop.Dir, "Service: api",
	}, identityLines(s.gui.snapshotConfigStr(snapshot), locationLabels...))
}

// Every Config tab's header says where its resource lives: the session's
// remote, the only one the resource panels list, and its project. A volume
// snapshot's says its volume's.
func TestEveryConfigHeaderSaysWhereItsResourceLives(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	header := func(config string) []string { return identityLines(config, locationLabels...) }
	want := []string{"Remote: fake", "Project: default"}

	image := onLoop(t, s, func() *commands.Image { return s.gui.Panels.Images.List.GetAllItems()[0] })
	assert.Equal(t, want, header(s.gui.imageConfigStr(image)))

	volume := onLoop(t, s, func() *commands.Volume { return s.gui.Panels.Volumes.List.GetAllItems()[0] })
	assert.Equal(t, want, header(s.gui.volumeConfigStr(volume)))

	network := onLoop(t, s, func() *commands.Network { return s.gui.Panels.Networks.List.GetAllItems()[0] })
	assert.Equal(t, want, header(s.gui.networkConfigStr(network)))

	profile := onLoop(t, s, func() *commands.Profile { return s.gui.Panels.Profiles.List.GetAllItems()[0] })
	assert.Equal(t, want, header(s.gui.profileConfigStr(profile)))
	assert.Len(t, identityLines(s.gui.profileConfigStr(profile), "Project"), 1, "the block's Project, not a second")

	snapshot := &commands.Snapshot{Project: volume.Volume.Project, Owner: volume.Name, Name: "nightly", Volume: volume}
	assert.Equal(t, want, header(s.gui.snapshotConfigStr(snapshot)))
}

// An instance that doesn't say which remote it came from is the session's,
// and a project two listed stacks share is the one saved first.
func TestLocationFallbacks(t *testing.T) {
	first := testStack(t, t.TempDir(), "shop", "api")
	second := testStack(t, t.TempDir(), "shop", "api")
	first.Remote, second.Remote = "fake", "fake"

	s := startScreenWith(t, 140, 40, nil, withStacks(t, nil, first, second))
	s.ready(t)

	require.Eventually(t, func() bool {
		return onLoop(t, s, func() int { return len(s.gui.Panels.Stacks.List.GetAllItems()) }) == 2
	}, 5*time.Second, 20*time.Millisecond)

	instance := &commands.Instance{Name: "api-1", Project: "shop", Instance: composeFixture("shop", "api-1", "api")}
	assert.Equal(t, location{remote: "fake", project: "shop", stack: first.Dir, service: "api"},
		s.gui.instanceLocation(instance))
}
