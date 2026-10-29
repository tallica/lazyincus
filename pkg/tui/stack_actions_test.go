package tui

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// typeInto fills the open prompt and submits it.
func typeInto(m *Model, text string) {
	m.topPrompt().input.SetValue(text)
	press(m, "enter")
}

func savedStacks(t *testing.T, m *Model) []string {
	t.Helper()

	state, err := m.cfg.LoadAppState()
	require.NoError(t, err)

	return state.Stacks
}

func TestAddingAStack(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	notCompose := t.TempDir()
	file := filepath.Join(notCompose, "notes.txt")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	m := newTestModel(t, options{compose: true})
	load := m.loadStack
	m.loadStack = func(dir string) *commands.ComposeStack {
		if dir == shop.Dir {
			loaded := *shop
			return &loaded
		}

		return load(dir)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	run(m, m.Init())
	press(m, "1")
	assert.Equal(t, m.tr.NoStacks, m.panelNamed("stacks").empty)

	for input, want := range map[string]string{
		filepath.Join(notCompose, "missing"): "no such file or directory",
		file:                                 "not a directory",
		notCompose:                           "isn't a compose project",
	} {
		_, err := m.saveStack(input, "")
		assert.ErrorContains(t, err, want, input)
	}

	// Relative to the working directory, and cleaned.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(cwd, shop.Dir)
	require.NoError(t, err)

	press(m, "a")
	assert.Equal(t, m.tr.AddStackPrompt, m.popups[0].title)
	typeInto(m, relative+"/.")

	assert.Equal(t, []string{"fake:" + shop.Dir}, savedStacks(t, m))
	assert.Equal(t, "stacks", m.currentView())
	assert.Equal(t, "shop", m.selectedStackRow().Name)
	assert.Empty(t, m.popups)

	_, err = m.saveStack(shop.Dir, "")
	assert.ErrorContains(t, err, "already listed")
}

// A refused add says why in a popup, once the status is off.
func TestAddingAMissingDirectoryIsRefused(t *testing.T) {
	m := start(t, options{compose: true}, 140, 40)

	press(m, "1", "a")
	typeInto(m, filepath.Join(t.TempDir(), "missing"))

	require.NotEmpty(t, m.popups)
	assert.Equal(t, m.tr.ErrorTitle, m.popups[0].title)
	assert.Contains(t, m.popups[0].message, "no such file or directory")
	assert.Empty(t, savedStacks(t, m))
}

// `a` takes a remote ahead of the directory, and only a remote it knows.
func TestAddingAStackOnARemote(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")

	m := newTestModel(t, options{compose: true, remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})}})
	m.loadStack = func(dir string) *commands.ComposeStack {
		loaded := *shop
		loaded.Dir = dir
		return &loaded
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	run(m, m.Init())

	save := func(input string) error {
		_, err := m.saveStack(input, "")
		return err
	}

	assert.ErrorContains(t, save("nope:"+shop.Dir), "no such file or directory")
	require.NoError(t, save("pve01:"+shop.Dir))
	assert.ErrorContains(t, save("pve01:"+shop.Dir), "already listed")
	// With no remote, it's pinned to the session's.
	require.NoError(t, save(shop.Dir))
	assert.ErrorContains(t, save("fake:"+shop.Dir), "already listed")
	assert.Equal(t, []string{"pve01:" + shop.Dir, "fake:" + shop.Dir}, savedStacks(t, m))

	// Removing it removes what was saved.
	run(m, refresh(m.fetchStacks()))
	require.Len(t, m.stacks, 2)
	require.NoError(t, m.cfg.RemoveStack(m.stacks[0].Ref()))
	assert.Len(t, savedStacks(t, m), 1)
}

func TestEditingAStack(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	shop.Remote = "fake"
	moved := t.TempDir()

	m := newTestModel(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{shop},
		remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})},
	})
	m.loadStack = func(dir string) *commands.ComposeStack {
		loaded := *shop
		loaded.Dir = dir
		return &loaded
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	run(m, m.Init())

	press(m, "e")
	assert.Equal(t, m.tr.EditStackPrompt, m.popups[0].title)
	assert.Equal(t, "fake:"+shop.Dir, m.popups[0].input.Value())
	press(m, "esc")

	// Unchanged is no change, and a directory that isn't there is refused.
	ref, err := m.saveStack("fake:"+shop.Dir, shop.Ref())
	require.NoError(t, err)
	assert.Empty(t, ref)

	_, err = m.saveStack("pve01:"+filepath.Join(moved, "missing"), shop.Ref())
	assert.ErrorContains(t, err, "no such file")

	press(m, "e")
	typeInto(m, "pve01:"+moved)
	assert.Equal(t, []string{"pve01:" + moved}, savedStacks(t, m))
	assert.Equal(t, "Services (shop on pve01)", m.servicesTitle())
}

// D on the local stack that's also saved forgets the entry and says the row stays, which it does.
func TestRemovingTheSavedEntryOfTheLocalStack(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	saved := *local
	saved.Remote = "fake"

	m := start(t, options{compose: true, localStack: local, savedStacks: []*commands.ComposeStack{&saved}}, 140, 40)

	press(m, "D")
	assert.Contains(t, m.popups[0].message, "It stays listed")
	press(m, "y")

	assert.Empty(t, savedStacks(t, m))
	require.Len(t, m.stacks, 1)
	assert.True(t, m.stacks[0].Local)
	assert.False(t, m.stacks[0].Saved)
}

// The local stack isn't saved, so there's nothing for D to remove.
func TestTheLocalStackCannotBeRemoved(t *testing.T) {
	m := start(t, options{compose: true, localStack: testStack(t, t.TempDir(), "default", "web")}, 140, 40)

	press(m, "D")
	assert.Equal(t, m.tr.ErrorTitle, m.popups[0].title)
	assert.Contains(t, m.popups[0].message, "It isn't saved")
}

func TestTheLocalStackCannotBeEdited(t *testing.T) {
	m := start(t, options{compose: true, localStack: testStack(t, t.TempDir(), "default", "web")}, 140, 40)

	press(m, "e")
	assert.Equal(t, m.tr.ErrorTitle, m.popups[0].title)
}

// y offers the project, the directory and each published port at its address.
func TestCopyingFromAStack(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")

	server := incustest.New(incustest.Server{})
	server.SetURL("https://192.0.2.5:8443")

	instance := composeFixture("default", "web", "web")
	instance.ExpandedConfig = maps.Clone(instance.ExpandedConfig)
	instance.ExpandedDevices = map[string]map[string]string{
		"proxy-80": {"type": "proxy", "listen": "tcp:0.0.0.0:8080", "connect": "tcp:127.0.0.1:80"},
	}
	server.SetInstances([]api.InstanceFull{instance})

	m := start(t, options{compose: true, server: server, localStack: stack}, 140, 40)

	press(m, "y")
	labels := lo.Map(m.topMenu().items, func(item *menuItem, _ int) string { return item.labelColumns[0] })
	assert.Contains(t, labels, "web → 80")
	assert.Contains(t, labels, m.tr.CopyProject)
	assert.Contains(t, labels, m.tr.CopyDirectory)
}

// Moving through the stacks moves nothing else; space on one moves the
// rest of the screen to its remote, and the list stays as it was.
func TestSpaceSwitchesToTheStacksRemote(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})
	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"
	there := testStack(t, t.TempDir(), "shop", "api")
	there.Remote = "pve01"
	// Saved before remotes: it follows the session, but keeps its place.
	plain := testStack(t, t.TempDir(), "zoo", "keeper")

	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{here, there, plain},
		remotes: map[string]*incustest.Server{"pve01": pve01},
	}, 140, 40)

	order := func() []string {
		return lo.Map(m.stacks, func(stack *commands.ComposeStack, _ int) string { return stack.Name })
	}

	// zoo was saved with no remote, so it sorts first.
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())

	press(m, "j", "j")
	require.Equal(t, "shop", m.selectedStackRow().Name)
	assert.Equal(t, "fake", m.incus.RemoteName())

	press(m, "space")
	assert.Equal(t, "pve01", m.incus.RemoteName())
	assert.Equal(t, []string{"api-1"}, lo.Map(m.instances, func(instance *commands.Instance, _ int) string { return instance.Name }))
	assert.Equal(t, []string{"zoo", "default", "shop"}, order())
	assert.Equal(t, "Services (shop)", m.servicesTitle())

	// Space on a stack already on the session's remote moves nothing.
	press(m, "space")
	assert.Equal(t, "pve01", m.incus.RemoteName())
	press(m, "k")
	press(m, "space")
	assert.Equal(t, "fake", m.incus.RemoteName())
}

// Space on a stack whose remote doesn't answer, or isn't known, leaves the screen where it was.
func TestSwitchingToAnUnreachableStack(t *testing.T) {
	t.Setenv("INCUS_REMOTE", "fake")

	down := testStack(t, t.TempDir(), "shop", "api")
	down.Remote = "down"
	gone := testStack(t, t.TempDir(), "old", "api")
	gone.Remote = "old-vm"

	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{down, gone}}, 140, 40)
	press(m, "1")

	require.Equal(t, "shop", m.selectedStackRow().Name)
	press(m, "space")
	require.NotEmpty(t, m.popups)
	assert.Contains(t, m.popups[0].message, "no answer")
	assert.Equal(t, "fake", m.incus.RemoteName())
	press(m, "esc", "j")

	require.Equal(t, "old", m.selectedStackRow().Name)
	press(m, "space")
	assert.Contains(t, m.popups[0].message, `no remote "old-vm"`)
	assert.Equal(t, "fake", m.incus.RemoteName())
}
