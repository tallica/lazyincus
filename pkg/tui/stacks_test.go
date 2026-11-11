package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// untilRemotes delivers what other remotes tell the model until done.
func untilRemotes(t *testing.T, m *Model, done func() bool) {
	t.Helper()

	for range 10 {
		if done() {
			return
		}

		deliver(t, m)
	}

	require.True(t, done(), "the remotes never settled")
}

func instanceNames(m *Model) []string {
	p := m.panelNamed("instances")

	return lo.Map(p.shown, func(item int, _ int) string { return m.instances.items[item].Name })
}

// Stacks is [1] and focused first, its listing reading the local stack.
func TestStacksAreFocusedFirst(t *testing.T) {
	root := t.TempDir()
	m := start(t, options{compose: true, localStack: testStack(t, root, "default", "web")}, 140, 40)

	assert.Equal(t, "stacks", m.currentView())
	assert.Contains(t, screen(m)[0], "[1]─Stacks")
	assert.Len(t, m.stacks.items, 1)
	assert.Equal(t, "default", m.stacks.selected().Name)
}

// Without incus-compose, there is no Stacks panel and nothing reads one.
func TestNoComposeNoStacks(t *testing.T) {
	m := start(t, options{}, 140, 40)

	assert.Nil(t, m.stacks.items)
	assert.Equal(t, "instances", m.currentView())
	assert.True(t, m.panelNamed("stacks").isHidden())
}

// With no stack on the session's remote, Stacks and Services collapse and
// the focus starts on Instances.
func TestNoStackHereCollapsesStacks(t *testing.T) {
	root := t.TempDir()
	pinned := testStack(t, root, "shop", "api")
	pinned.Remote = "pve01"
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})

	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{pinned}, remotes: map[string]*incustest.Server{"pve01": pve01}}, 140, 40)

	assert.Equal(t, "instances", m.currentView())
	assert.False(t, m.stacksHere)
	assert.Contains(t, screen(m)[0], "[1]─Stacks (1)")

	press(m, "1")
	assert.Equal(t, "stacks", m.currentView())
}

// With collapseStacksElsewhere off, a remote with no stack keeps the even
// split and the focus on Stacks.
func TestNoStackHereWithoutCollapse(t *testing.T) {
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned},
		remotes:   map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})},
		configure: func(userConfig *config.UserConfig) { userConfig.Gui.CollapseStacksElsewhere = false },
	}, 140, 40)

	assert.Equal(t, "stacks", m.currentView())
	assert.Contains(t, screen(m)[0], "[1]─Stacks─")
}

// A popup opened before the stacks are first read hands the focus to
// Instances when it closes, not back to Stacks.
func TestNoStackHereLandsPastAPopup(t *testing.T) {
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	m := newTestModel(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned},
		remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})},
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	assert.Equal(t, "stacks", m.currentStaticView())

	m.openMessage("Title", "Message")
	run(m, m.Init())
	assert.Equal(t, "Title", m.popups[0].title)

	press(m, "esc")
	assert.Equal(t, "instances", m.currentView())
}

// Every listed stack's compose instances are the stacks' own; one of a
// project no stack lists, or of a service its stack no longer declares,
// stays standalone. C puts them back.
func TestStandaloneInstancesLeaveOutEveryStack(t *testing.T) {
	instances := incustest.Fixture().Instances
	compose := func(project, name, service string) api.InstanceFull {
		instance := instances[0]
		instance.Name, instance.Project = name, project
		instance.ExpandedConfig = map[string]string{"user.label.incus-compose.service": service}

		return instance
	}

	root := t.TempDir()
	server := incustest.New(incustest.Server{Instances: append(instances[:0:0],
		compose("default", "web", "web"),
		compose("shop", "api-1", "api"),
		compose("shop", "worker-1", "worker"),
		compose("elsewhere", "job-1", "job"),
		instances[1],
	)})

	m := start(t, options{
		compose: true, server: server,
		savedStacks: []*commands.ComposeStack{testStack(t, root, "default", "web"), testStack(t, root, "shop", "api")},
	}, 140, 40)

	shows := func() []string {
		names := instanceNames(m)
		slices.Sort(names)

		return names
	}

	assert.Equal(t, []string{"db", "job-1", "worker-1"}, shows())
	assert.Equal(t, m.tr.StandaloneInstancesTitle, m.instancesTitle())

	press(m, "3", "C")
	assert.Equal(t, []string{"api-1", "db", "job-1", "web", "worker-1"}, shows())
	assert.Equal(t, m.tr.InstancesTitle, m.instancesTitle())

	press(m, "C")
	assert.Equal(t, []string{"db", "job-1", "worker-1"}, shows())
	assert.Equal(t, m.tr.StandaloneInstancesTitle, m.instancesTitle())
}

// The project column is decided over the instances left standalone.
func TestStackFilterNarrowsTheProjectSpan(t *testing.T) {
	root := t.TempDir()
	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		composeFixture("shop", "api-1", "api"),
		composeFixture("default", "db", ""),
	}})

	m := start(t, options{compose: true, server: server, savedStacks: []*commands.ComposeStack{testStack(t, root, "shop", "api")}}, 140, 40)
	assert.False(t, m.instances.span)

	press(m, "3", "C")
	assert.True(t, m.instances.span)
}

// A stack pinned to another remote is that remote's: its statuses come
// from a read off to one side, and show once it lands.
func TestAStackPinnedToARemote(t *testing.T) {
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})

	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned}, remotes: map[string]*incustest.Server{"pve01": pve01},
	}, 140, 40)

	assert.True(t, m.stacks.items[0].StatusPending)
	assert.True(t, m.stacksElsewhere)

	untilRemotes(t, m, func() bool { return !m.stacks.items[0].StatusPending })
	assert.Equal(t, "Running", m.stacks.items[0].Status())
	assert.Regexp(t, `pve01 +shop`, strings.Join(rowsOf(m, "stacks"), "\n"))
}

// A remote that doesn't answer marks its own stacks, and only those.
func TestAStackOnAnUnreachableRemote(t *testing.T) {
	unreachable := testStack(t, t.TempDir(), "shop", "api")
	unreachable.Remote = "down"
	local := testStack(t, t.TempDir(), "default", "web")

	m := start(t, options{
		compose: true, localStack: local, savedStacks: []*commands.ComposeStack{unreachable},
	}, 140, 40)

	untilRemotes(t, m, func() bool { return m.stacks.items[1].StatusErr != nil })

	assert.Equal(t, []string{"none", "unreachable"}, lo.Map(m.stacks.items, func(stack *commands.ComposeStack, _ int) string {
		if stack.StatusErr != nil {
			return "unreachable"
		}

		return stack.Status()
	}))
	assert.Empty(t, m.popups)
}

// A stack whose remote has gone from the CLI's config says so, rather than trying to connect.
func TestAStackOnARemoteThatIsGone(t *testing.T) {
	gone := testStack(t, t.TempDir(), "shop", "api")
	gone.Remote = "old-vm"

	m := newTestModel(t, options{compose: true, savedStacks: []*commands.ComposeStack{gone}})
	connect := m.remotes.connect
	m.remotes.connect = func(remote string) (*commands.IncusCommand, error) {
		assert.NotEqual(t, "old-vm", remote)
		return connect(remote)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	run(m, m.Init())

	untilRemotes(t, m, func() bool { return m.stacks.items[0].StatusErr != nil })
	assert.ErrorContains(t, m.stacks.items[0].StatusErr, `no remote "old-vm" in the incus CLI's config`)
}

// The local stack saved too is one row, which the saved entry's remote names.
func TestTheLocalStackSavedIsOneRow(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	saved, elsewhere := *local, *local
	saved.Remote, elsewhere.Remote = "fake", "pve01"

	m := start(t, options{
		compose: true, localStack: local, savedStacks: []*commands.ComposeStack{&saved, &elsewhere},
		remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})},
	}, 140, 40)

	require.Len(t, m.stacks.items, 2)
	assert.True(t, m.stacks.items[0].Local && m.stacks.items[0].Saved)
	assert.Equal(t, "fake:"+local.Dir, m.stacks.items[0].Ref())
	assert.Equal(t, "pve01:"+local.Dir, m.stacks.items[1].Ref())
}

// A slow remote holds up nothing: the others show at once, its stack connecting.
func TestASlowRemoteHoldsUpNothing(t *testing.T) {
	here := testStack(t, t.TempDir(), "default", "web")
	here.Remote = "fake"
	slow := testStack(t, t.TempDir(), "shop", "api")
	slow.Remote = "pve01"

	answer := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-answer:
		default:
			close(answer)
		}
	})

	m := newTestModel(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{here, slow},
		server:  incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("default", "web", "web")}}),
		remotes: map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})},
	})
	connect := m.remotes.connect
	m.remotes.connect = func(remote string) (*commands.IncusCommand, error) {
		if remote == "pve01" {
			<-answer
		}

		return connect(remote)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	run(m, m.Init())

	status := func(name string) string {
		for _, stack := range m.stacks.items {
			if stack.Name == name {
				if stack.StatusPending {
					return "connecting"
				}

				return stack.Status()
			}
		}

		return ""
	}

	assert.Equal(t, "Running", status("default"))
	assert.Equal(t, "connecting", status("shop"))
	assert.NotEmpty(t, m.instances.items)

	close(answer)
	untilRemotes(t, m, func() bool { return status("shop") == "Running" })
}

// A remote whose error is worded differently every time refreshes the stacks
// when it starts failing, not on every read: each refresh reads again.
func TestAChangingErrorRefreshesOnce(t *testing.T) {
	var remotes remoteCommands

	var reads, changes atomic.Int32

	read := func() (map[string]commands.ComposeProjectState, error) {
		return map[string]commands.ComposeProjectState{}, fmt.Errorf("refused, attempt %d", reads.Add(1))
	}

	var changed func()

	changed = func() {
		changes.Add(1)
		remotes.cachedStatuses("pve01", read, changed)
	}

	remotes.cachedStatuses("pve01", read, changed)

	require.Eventually(t, func() bool { return reads.Load() == 2 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(2), reads.Load())
	assert.Equal(t, int32(1), changes.Load())
	assert.EqualError(t, remotes.remoteStatusErr("pve01"), "refused, attempt 2")
}

// While the stream is live the stacks poll waits a minute, unless a stack is on another remote.
func TestTheStacksPollWatchesOtherRemotes(t *testing.T) {
	m := start(t, options{compose: true}, 140, 40)
	m.eventsLive = true
	recent := time.Now().Add(-time.Second)
	m.polledAt = map[refreshKind]time.Time{refreshStacks: recent}

	m.Update(slowPollMsg{refreshStacks})
	assert.Equal(t, recent, m.polledAt[refreshStacks])

	m.stacksElsewhere = true
	m.Update(slowPollMsg{refreshStacks})
	assert.True(t, m.polledAt[refreshStacks].After(recent))
}

const hashKey = "user.label.incus-compose.config-hash"

// A service whose instance was created from another config than the file
// declares reads changed on its row, its stack's, and the stack's Info tab.
func TestAChangedServiceIsMarked(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	local.Services[0].Hash = "v1:new"

	web := composeFixture("default", "web", "web")
	web.ExpandedConfig[hashKey] = "v1:old"
	server := incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{web}})

	m := start(t, options{compose: true, server: server, localStack: local}, 140, 40)
	text := strings.Join(screen(m), "\n")

	assert.Contains(t, text, "│default running ↻")
	assert.Contains(t, text, "│web running ↻")
	assert.Contains(t, text, "1 instance from an older config, recreate with u")
}

// An edit to a listed stack's compose file, which no daemon event
// announces, reloads the stack at the next stat and re-marks its services.
func TestAnEditedComposeFileReloadsTheStack(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	local.Services[0].Hash = "v1:old"
	file := filepath.Join(local.Dir, "compose.yaml")
	require.NoError(t, os.WriteFile(file, []byte("services: {web: {}}\n"), 0o644))

	web := composeFixture("default", "web", "web")
	web.ExpandedConfig[hashKey] = "v1:old"
	server := incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{web}})

	m := start(t, options{compose: true, server: server, localStack: local}, 140, 40)
	require.NotContains(t, strings.Join(screen(m), "\n"), "changed")

	poll := func() {
		_, cmd := m.Update(stackFilesPollMsg{})
		run(m, cmd)
	}

	loads := 0
	load := m.loadStack
	m.loadStack = func(dir string) *commands.ComposeStack {
		loads++
		stack := load(dir)
		stack.Services = slices.Clone(stack.Services)
		stack.Services[0].Hash = "v1:new"

		return stack
	}

	poll()
	assert.Zero(t, loads, "an unchanged file is not read again")

	require.NoError(t, os.WriteFile(file, []byte("services: {web: {image: nginx}}\n"), 0o644))
	poll()
	assert.Equal(t, 1, loads)
	assert.Contains(t, strings.Join(screen(m), "\n"), "│web running ↻")
}

// A cached stack is read again once its compose file changes on disk, and
// not before.
func TestStackCacheReloadsAnEditedFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(file, []byte("services: {}\n"), 0o644))

	loads := 0
	load := func(dir string) *commands.ComposeStack {
		loads++
		return &commands.ComposeStack{Dir: dir}
	}

	var cache stackCache

	cache.get(dir, false, load)
	cache.get(dir, false, load)
	assert.Equal(t, 1, loads)

	assert.False(t, cache.edited([]string{dir, t.TempDir()}))

	require.NoError(t, os.WriteFile(file, []byte("services: {web: {}}\n"), 0o644))
	assert.True(t, cache.edited([]string{dir}))

	cache.get(dir, false, load)
	assert.Equal(t, 2, loads)
	assert.False(t, cache.edited([]string{dir}))
}
