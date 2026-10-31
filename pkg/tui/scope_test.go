package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// R lists the remotes with instances, the session's marked and selected.
func TestTheRemotesMenu(t *testing.T) {
	m := start(t, options{compose: true, remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})}}, 140, 40)

	press(m, "R")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, m.tr.RemotesTitle, m.topMenu().title)

	assert.Regexp(t, `\*\s+fake`, strings.Join(menuRows(m, screen(m)), "\n"))
	assert.Equal(t, []string{"*", "fake"}, m.topMenu().selectedItem().labelColumns)
}

func TestTheRemotesMenuStartsOnTheSessions(t *testing.T) {
	m := start(t, options{compose: true, remotes: map[string]*incustest.Server{"alpha": incustest.New(incustest.Server{})}}, 140, 40)

	press(m, "R")
	assert.Equal(t, []string{"*", "fake"}, m.topMenu().selectedItem().labelColumns)
}

// Switching moves every panel, the footer and the shell-outs onto the new
// remote, and a stack pinned to the one left keeps reading from it.
func TestSwitchingRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	pve01 := incustest.New(incustest.Server{Version: "7.5", Instances: []api.InstanceFull{composeFixture("default", "only-on-pve01", "")}})
	pinned := testStack(t, t.TempDir(), "default", "web")
	pinned.Remote = "fake"

	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned}, remotes: map[string]*incustest.Server{"pve01": pve01},
		server: incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("default", "web", "web")}}),
	}, 140, 40)

	assert.Equal(t, "stacks", m.currentView())
	assert.Equal(t, []string{"web"}, instanceNamesIn(m, "services"))

	run(m, m.switchToRemote("pve01"))

	assert.Equal(t, "pve01", m.incus.RemoteName())
	assert.Equal(t, "pve01", os.Getenv("INCUS_REMOTE"))
	assert.Equal(t, []string{"only-on-pve01"}, instanceNames(m))
	assert.Contains(t, screen(m)[39], "pve01/all projects")

	// No stack is on pve01, so the focus leaves Stacks for Instances.
	assert.Equal(t, "instances", m.currentView())
	assert.False(t, m.stacksHere)

	// fake is a remote like any other now, and still answers for its stack.
	press(m, "1")
	untilRemotes(t, m, func() bool { return len(m.serviceRows) == 1 && m.stacks[0].Status() == "Running" })
	assert.Equal(t, "Services (default on fake)", m.servicesTitle())
}

func instanceNamesIn(m *Model, panel string) []string {
	if panel == "services" {
		names := make([]string, len(m.serviceRows))
		for i, row := range m.serviceRows {
			names[i] = row.Key()
		}

		return names
	}

	return nil
}

// A switch keeps the project the panels are scoped to, where the remote has one by that name.
func TestSwitchingRemoteKeepsTheProject(t *testing.T) {
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "web", "")}})
	edge := incustest.New(incustest.Server{})

	m := start(t, options{compose: true, remotes: map[string]*incustest.Server{"pve01": pve01, "edge": edge}}, 140, 40)

	run(m, m.switchToProject("shop"))
	assert.Contains(t, screen(m)[39], "(fake/shop)")

	run(m, m.switchToRemote("pve01"))
	assert.Contains(t, screen(m)[39], "(pve01/shop)")

	run(m, m.switchToRemote("edge"))
	assert.Contains(t, screen(m)[39], "(edge/all projects)")
}

// A remote that doesn't answer leaves everything where it was.
func TestSwitchingToAnUnreachableRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	m := start(t, options{compose: true}, 140, 40)
	names := instanceNames(m)

	run(m, m.switchToRemote("down"))

	require.NotEmpty(t, m.popups)
	assert.Contains(t, m.popups[0].message, "no answer")
	assert.Equal(t, "fake", m.incus.RemoteName())
	assert.Equal(t, "fake", os.Getenv("INCUS_REMOTE"))
	assert.Equal(t, names, instanceNames(m))
}

// P lists the projects with every project first, and switching narrows Instances.
func TestProjectsMenu(t *testing.T) {
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		composeFixture("default", "db", ""), composeFixture("shop", "web", ""),
	}})
	m := start(t, options{server: server}, 140, 40)

	press(m, "P")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, m.tr.ProjectsTitle, m.topMenu().title)
	assert.Equal(t, []string{"*", m.tr.AllProjects}, m.topMenu().items[0].labelColumns)

	press(m, "j", "j", "enter")
	assert.Equal(t, "shop", m.incus.ProjectName())
	assert.Equal(t, []string{"web"}, instanceNames(m))

	press(m, "P", "k", "k", "enter")
	assert.True(t, m.incus.IsAllProjects())
	assert.Len(t, instanceNames(m), 2)
}

// Names that land after an R are the old remote's: no menu of them.
func TestProjectsMenuAfterARemoteSwitch(t *testing.T) {
	m := start(t, options{compose: true, remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})}}, 140, 40)

	names := m.switchProjectMenu()
	run(m, m.switchToRemote("pve01"))
	run(m, names)

	assert.Nil(t, m.topMenu())
}
