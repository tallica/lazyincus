package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func serviceNames(m *Model) []string {
	return lo.Map(m.serviceRows, func(row *commands.ServiceRow, _ int) string { return row.Service.Project + "/" + row.Key() })
}

// withSnapshot is instance with one snapshot of its own.
func withSnapshot(instance api.InstanceFull, name string) api.InstanceFull {
	instance.Snapshots = []api.InstanceSnapshot{{
		Name: instance.Name + "/" + name, CreatedAt: time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC),
	}}

	return instance
}

// The Services panel follows the Stacks panel's selection, and a fetch
// still out for the stack it left can't bring that one's rows back.
func TestServicesFollowTheSelectedStack(t *testing.T) {
	root := t.TempDir()
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		composeFixture("default", "web", "web"), composeFixture("shop", "api", "api"),
	}})

	m := start(t, options{
		compose: true, server: server,
		localStack: testStack(t, root, "default", "web"), savedStacks: []*commands.ComposeStack{testStack(t, root, "shop", "api")},
	}, 140, 40)

	assert.Equal(t, "Services (default)", m.servicesTitle())
	assert.Equal(t, []string{"default/web"}, serviceNames(m))

	// Fetched for default, then overtaken by the move to shop.
	apply, err := m.fetchServices()()
	require.NoError(t, err)

	press(m, "j")
	assert.Equal(t, "Services (shop)", m.servicesTitle())

	apply()
	assert.NotContains(t, serviceNames(m), "default/web")
	assert.Equal(t, []string{"shop/api"}, serviceNames(m))
}

// No stack selected says so, where a stack with no services says that.
func TestServicesSayWhyTheyAreEmpty(t *testing.T) {
	m := start(t, options{compose: true}, 140, 40)

	assert.Equal(t, m.tr.NoStackSelected, m.panelNamed("services").empty)
	assert.Equal(t, m.tr.ServicesTitle, m.servicesTitle())

	root := t.TempDir()
	m = start(t, options{compose: true, localStack: testStack(t, root, "default", "web")}, 140, 40)
	assert.Equal(t, m.tr.NoServices, m.panelNamed("services").empty)
}

// A replicated service lists its replicas under it, in order, and the
// cursor stays on its row across a refresh.
func TestAReplicatedServiceListsItsReplicas(t *testing.T) {
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		composeFixture("default", "web-2", "web"), composeFixture("default", "web-1", "web"), composeFixture("default", "db-1", "db"),
	}})

	m := start(t, options{compose: true, server: server, localStack: testStack(t, t.TempDir(), "default", "web", "db")}, 140, 40)

	assert.Equal(t, []string{"default/db", "default/web", "default/web/web-1", "default/web/web-2"}, serviceNames(m))

	press(m, "2", "j", "j", "j")
	require.Equal(t, "web/web-2", m.selectedServiceRow().Key())

	run(m, refresh(m.fetchServices()))
	assert.Equal(t, "web/web-2", m.selectedServiceRow().Key())
}

// A stack on a remote that isn't answering empties the list, with a note
// saying so, and says why in main.
func TestAStackOnAnUnreachableRemoteHasNoServices(t *testing.T) {
	unreachable := testStack(t, t.TempDir(), "shop", "api")
	unreachable.Remote = "down"

	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{unreachable}}, 140, 40)

	untilRemotes(t, m, func() bool { return m.stacks[0].StatusErr != nil })
	run(m, refresh(m.fetchServices()))
	assert.Equal(t, fmt.Sprintf(m.tr.CannotReachRemote, "down"), m.servicesNote)
	assert.Equal(t, []string{m.servicesNote}, rowsOf(m, "services"))
	assert.Contains(t, m.panelNamed("services").empty, "no answer")
	assert.Empty(t, m.popups)
}

// A stack elsewhere lists that remote's services, and Services says where.
func TestAStackPinnedToARemoteHasItsServices(t *testing.T) {
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})

	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned}, remotes: map[string]*incustest.Server{"pve01": pve01},
		server: incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "web", "api")}}),
	}, 140, 40)

	untilRemotes(t, m, func() bool { return len(m.serviceRows) > 0 })
	assert.Equal(t, "Services (shop on pve01)", m.servicesTitle())
	assert.Equal(t, []string{"shop/api"}, serviceNames(m))

	instance := m.serviceRows[0].Service.Instances[0]
	assert.Equal(t, "api-1", instance.Name)
	assert.Equal(t, "pve01", instance.Remote)

	// The session's own instance of that project stays standalone.
	assert.Contains(t, instanceNames(m), "web")
}

// Snapshots follows a service's row - every replica's - and a replica's.
func TestSnapshotsFollowAServicesRows(t *testing.T) {
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		withSnapshot(composeFixture("default", "web-1", "web"), "one"),
		withSnapshot(composeFixture("default", "web-2", "web"), "two"),
	}})

	m := start(t, options{compose: true, server: server, localStack: testStack(t, t.TempDir(), "default", "web")}, 140, 40)

	press(m, "2")
	require.Equal(t, "web", m.selectedServiceRow().Key())
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())
	assert.Equal(t, []string{"one web-1", "two web-2"}, snapshotRows(m))

	press(m, "j")
	assert.Equal(t, "Snapshots (web-1)", m.snapshotsTitle())
	assert.Equal(t, []string{"one 2026/09/20"}, snapshotRows(m))
}

func snapshotRows(m *Model) []string {
	rows := rowsOf(m, "snapshots")
	slices.Sort(rows)

	return lo.Map(rows, func(row string, _ int) string {
		return strings.Join(strings.Fields(row)[:min(2, len(strings.Fields(row)))], " ")
	})
}

// n on Snapshots, following a replicated service, asks which replica.
func TestNewSnapshotOfAServiceAsksWhichReplica(t *testing.T) {
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"),
	}})

	m := start(t, options{compose: true, server: server, localStack: testStack(t, t.TempDir(), "default", "web")}, 140, 40)

	press(m, "2")
	require.Equal(t, "web", m.selectedServiceRow().Key())
	press(m, "4", "n")

	require.NotNil(t, m.topMenu())
	labels := lo.Map(m.topMenu().items, func(item *menuItem, _ int) string { return item.label })
	assert.ElementsMatch(t, []string{"web-1", "web-2"}, lo.Compact(labels))
}
