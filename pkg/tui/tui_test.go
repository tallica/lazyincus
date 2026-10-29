package tui

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

type options struct {
	readOnly bool
	compose  bool
	// fixture is pkg/gui's screen-test daemon in place of the small one.
	fixture   bool
	configure func(*config.UserConfig)
	// server is the daemon, for a test that changes it, in place of either.
	server *incustest.Server
	// localStack is the stack lazyincus started in, savedStacks those in state.yml.
	localStack  *commands.ComposeStack
	savedStacks []*commands.ComposeStack
	// remotes are other daemons by CLI remote name; "down" is known but never answers.
	remotes map[string]*incustest.Server
}

func newTestModel(t *testing.T, opts options) *Model {
	t.Helper()

	userConfig := config.GetDefaultConfig()
	if opts.configure != nil {
		opts.configure(&userConfig)
	}
	appConfig := &config.AppConfig{
		Name: "lazyincus", Version: "test", UserConfig: &userConfig, ConfigDir: t.TempDir(), ReadOnly: opts.readOnly,
	}

	instance := func(name, status string) api.InstanceFull {
		return api.InstanceFull{Instance: api.Instance{Name: name, Project: "default", Status: status, Type: "container"}}
	}

	log := commands.NewDummyLog()
	tr := i18n.NewTranslationSet(log, "en")
	osCommand := commands.NewOSCommand(log, appConfig)
	server := incustest.New(incustest.Server{
		Version:   "7.4",
		Instances: []api.InstanceFull{instance("db", "Stopped"), instance("web", "Running"), instance("cache", "Running")},
	})
	if opts.fixture {
		server = incustest.Fixture()
	}
	if opts.server != nil {
		server = opts.server
	}
	incus := commands.NewIncusCommandWithClient(log, osCommand, tr, appConfig, server, "fake")

	m := NewModel(incus, osCommand, tr, appConfig)
	// Ticks never fire on their own: a test sends the message itself.
	m.after = func(time.Duration, tea.Msg) tea.Cmd { return nil }
	m.streamEvents = false
	m.runTask = runTaskHere
	m.checkUpstreams = func(context.Context, []commands.UpstreamTarget) map[string]commands.UpstreamStatus { return nil }
	// Whatever this machine has installed, the panels are the test's.
	m.composeAvailable = opts.compose
	m.panels = m.newPanels()
	m.focus = nil
	if opts.compose {
		withStacks(t, m, opts.localStack, opts.savedStacks...)
		withRemotes(m, server, opts.remotes)
	}
	m.landOffStacks()

	return m
}

// start sizes m and feeds it its first fetch, the way the program would.
func start(t *testing.T, opts options, width, height int) *Model {
	t.Helper()

	m := newTestModel(t, opts)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	run(m, m.Init())

	return m
}

// run runs cmd and every Cmd its messages lead to, in order, as the program
// would but on this goroutine.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}

	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, cmd := range msg {
			run(m, cmd)
		}
	default:
		_, next := m.Update(msg)
		run(m, next)
	}
}

// runTaskHere runs a main task's first pass on the Cmd's goroutine, its
// writes the messages that follow: a ticker stops after its first.
func runTaskHere(task func(ctx context.Context, send func(tea.Msg))) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var writes tea.BatchMsg
		task(ctx, func(msg tea.Msg) {
			writes = append(writes, func() tea.Msg { return msg })
		})

		return writes
	}
}

// refreshNow is a keypress's refresh of the instances, run to the end.
func refreshNow(m *Model) {
	run(m, refresh(m.fetchInstances()))
}

// press types keys by name, as Update would take them from KeyPressMsg.
func press(m *Model, keys ...string) {
	for _, key := range keys {
		_, cmd := m.Update(keyPress(key))
		run(m, cmd)
	}
}

var namedKeys = map[string]rune{
	"esc": tea.KeyEscape, "enter": tea.KeyEnter, "tab": tea.KeyTab, "space": tea.KeySpace,
	"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

// keyPress is the KeyPressMsg whose String is name: "x", "esc", "ctrl+u".
func keyPress(name string) tea.KeyPressMsg {
	msg := tea.KeyPressMsg{}
	rest := name
	for _, mod := range []struct {
		prefix string
		mod    tea.KeyMod
	}{{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift}} {
		if after, ok := strings.CutPrefix(rest, mod.prefix); ok && after != "" {
			msg.Mod |= mod.mod
			rest = after
		}
	}

	if code, ok := namedKeys[rest]; ok {
		msg.Code = code
		// A terminal types space as it does any rune.
		if code == tea.KeySpace && msg.Mod == 0 {
			msg.Text = " "
		}
	} else {
		msg.Code = []rune(rest)[0]
		if msg.Mod == 0 {
			msg.Text = rest
		}
	}

	if msg.String() != name {
		panic("no KeyPressMsg reads " + name)
	}

	return msg
}

// screen is m's screen as plain text, each line padded to the width.
func screen(m *Model) []string {
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = fit(line, m.width)[:len(fit(line, m.width))-len(reset)]
	}

	return lines
}

func TestScreenFillsTheTerminal(t *testing.T) {
	m := start(t, options{}, 90, 16)

	lines := screen(m)
	require.Len(t, lines, 16)
	for _, line := range lines {
		assert.Equal(t, 90, ansi.StringWidth(line), line)
	}

	assert.Contains(t, lines[0], "[1]─Instances")
	assert.Contains(t, lines[0], "Info - Logs - Config - Env - Top")
	assert.Contains(t, lines[1], "cache", "running instances come first, by name")
	assert.Contains(t, lines[3], "db")
}

func TestSelectionFollowsTheInstanceAcrossRefresh(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "j")
	require.Equal(t, "web", m.selectedInstance().Name)

	refreshNow(m)
	assert.Equal(t, "web", m.selectedInstance().Name)
}

func TestLateFetchIsDropped(t *testing.T) {
	server := incustest.New(incustest.Server{Version: "7.4"})
	server.SetInstances([]api.InstanceFull{{Instance: api.Instance{Name: "old", Project: "default", Status: "Running"}}})
	m := start(t, options{server: server}, 90, 16)

	late := refresh(m.fetchInstances())()
	server.SetInstances([]api.InstanceFull{{Instance: api.Instance{Name: "new", Project: "default", Status: "Running"}}})
	refreshNow(m)
	m.Update(late)

	require.Len(t, m.instances, 1)
	assert.Equal(t, "new", m.instances[0].Name)
}

func TestPanelsSayLoadingUntilRefreshAllLands(t *testing.T) {
	m := newTestModel(t, options{})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 16})

	assert.Contains(t, screen(m)[1], "Loading…")
	run(m, m.refreshAll())
	assert.Contains(t, screen(m)[1], "cache")
	for _, p := range m.panels {
		assert.Empty(t, p.awaiting, p.name)
	}
}

func TestAnUnreachableDaemonIsNoErrorPopup(t *testing.T) {
	server := incustest.New(incustest.Server{Version: "7.4"})
	m := start(t, options{server: server}, 90, 16)

	server.SetDown(true)
	refreshNow(m)
	assert.Empty(t, m.popups, "an unreachable daemon is the footer's to report")
	assert.False(t, m.incus.IsConnected())
}

func TestExecRefusedOnReadOnlyInAnErrorPopup(t *testing.T) {
	m := start(t, options{readOnly: true}, 90, 16)

	press(m, "E")
	require.Len(t, m.popups, 1)
	assert.Contains(t, strings.Join(screen(m), "\n"), "--read-only")
	assert.Contains(t, screen(m)[15], "esc/enter: close")

	press(m, "esc")
	assert.Empty(t, m.popups)
}

func TestTabsSwitchTheMainPanel(t *testing.T) {
	m := start(t, options{}, 90, 16)

	assert.Contains(t, strings.Join(screen(m), "\n"), "Name:")
	press(m, "]", "]")
	assert.Contains(t, strings.Join(screen(m), "\n"), "name: cache")
}

func TestNumberKeysFocusWindowsAndCycleTheirTabs(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "3")
	assert.Equal(t, "images", m.currentView())
	press(m, "3")
	assert.Equal(t, "volumes", m.currentView())
	assert.Contains(t, screen(m)[26], "Images - Volumes - Networks - Profiles")

	press(m, "1")
	press(m, "3")
	assert.Equal(t, "volumes", m.currentView(), "a window remembers the tab it was left on")
}

func TestTabCyclesWindowsAndArrowsCycleLists(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "tab", "tab")
	assert.Equal(t, "images", m.currentView())
	press(m, "tab")
	assert.Equal(t, "instances", m.currentView())

	press(m, "shift+tab", "l")
	assert.Equal(t, "volumes", m.currentView())
	press(m, "h", "h")
	assert.Equal(t, "snapshots", m.currentView())
}

func TestMainPanelTakesFocusAndGivesItBack(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "enter")
	assert.Equal(t, mainView, m.currentView())
	assert.Equal(t, "instances", m.currentSideView())

	press(m, "tab")
	assert.Equal(t, "snapshots", m.currentView(), "tab carries on from the last list")

	press(m, "enter", "esc")
	assert.Equal(t, "snapshots", m.currentView())
}

func TestConfirmOnQuitAsks(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) { c.ConfirmOnQuit = true }}, 140, 40)

	assert.Nil(t, m.handleKey(keyPress("q")))
	require.Len(t, m.popups, 1)
	assert.Contains(t, screen(m)[39], "n/esc: no, y/enter: yes")

	cmd := m.handleKey(keyPress("y"))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestTooSmall(t *testing.T) {
	m := start(t, options{}, 9, 20)

	assert.Contains(t, strings.Join(screen(m), ""), "Not")
}

func TestMainScrollKeys(t *testing.T) {
	m := start(t, options{}, 90, 9)
	press(m, "]", "]")
	step := m.cfg.UserConfig.Gui.ScrollHeight
	require.Contains(t, screen(m)[1], "architecture:")

	press(m, "J")
	assert.Equal(t, step, m.main.oy)
	assert.NotContains(t, screen(m)[1], "architecture:")

	press(m, "home")
	assert.Contains(t, screen(m)[1], "architecture:")

	press(m, "end")
	assert.Equal(t, len(m.main.lines)-6, m.main.oy, "autoscroll keeps the last line in view")
	press(m, "K")
	assert.False(t, m.main.autoscroll)
	assert.Equal(t, len(m.main.lines)-6-step, m.main.oy)
}

func TestEscFromMainReturnsToTheFirstList(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "enter")
	assert.Equal(t, mainView, m.currentView())
	press(m, "esc")
	assert.Equal(t, "instances", m.currentView())
}

func TestAnEmptyListLeavesItsMessageToMain(t *testing.T) {
	m := start(t, options{}, 140, 40)
	press(m, "2")

	snapshots, ok := m.dims()[snapshotsWindow]
	require.True(t, ok)
	assert.NotContains(t, strings.Join(inside(screen(m), snapshots), ""), "No snapshots")

	main := strings.Join(inside(screen(m), m.dims()[mainView]), "\n")
	assert.Contains(t, main, "No snapshots")
	assert.NotContains(t, screen(m)[0], "Info - Logs - Config - Env - Top", "an empty list's main has no tabs")
}

// testStack is a stack in a directory of its own under root, named for its
// Incus project and declaring services.
func testStack(t *testing.T, root, project string, services ...string) *commands.ComposeStack {
	t.Helper()

	dir := filepath.Join(root, project)
	require.NoError(t, os.Mkdir(dir, 0o755))

	stack := &commands.ComposeStack{Dir: dir, Name: project}
	for _, service := range services {
		stack.Services = append(stack.Services, commands.ComposeService{Name: service, Replicas: 1})
	}

	return stack
}

// withStacks lists stacks without incus-compose: local is the stack
// lazyincus started in, nil for none, and saved are in state.yml. A
// directory neither names reads as having no compose file.
func withStacks(t *testing.T, m *Model, local *commands.ComposeStack, saved ...*commands.ComposeStack) {
	t.Helper()

	byDir := map[string]*commands.ComposeStack{}

	m.localStackDir, m.localStackExplicit = "", false
	if local != nil {
		m.localStackDir = local.Dir
		byDir[local.Dir] = local
	}

	for _, stack := range saved {
		require.NoError(t, m.cfg.AddStack(stack.Ref()))
		byDir[stack.Dir] = stack
	}

	m.loadStack = func(dir string) *commands.ComposeStack {
		if stack, ok := byDir[dir]; ok {
			loaded := *stack
			return &loaded
		}

		return &commands.ComposeStack{Dir: dir, Err: errors.New("no compose.yaml found")}
	}
}

// withRemotes stands each server in for the CLI remote it's keyed by, the
// session's own for "fake", and "down" for a remote that's known but
// doesn't answer.
func withRemotes(m *Model, session *incustest.Server, servers map[string]*incustest.Server) {
	servers = maps.Clone(servers)
	if servers == nil {
		servers = map[string]*incustest.Server{}
	}
	servers["fake"] = session

	names := append(slices.Sorted(maps.Keys(servers)), "down")
	slices.Sort(names)

	m.remotes.names = func() []string { return names }
	m.remotes.known = func(remote string) bool { return slices.Contains(names, remote) }
	m.remotes.connect = func(remote string) (*commands.IncusCommand, error) {
		server, ok := servers[remote]
		if !ok {
			return nil, &commands.ConnectError{Remote: remote, Err: errors.New("no answer")}
		}

		return commands.NewIncusCommandWithClient(m.incus.Log, m.incus.OSCommand, m.incus.Tr, m.incus.Config, server, remote), nil
	}
}

// composeFixture is a fixture instance of project that incus-compose made for service.
func composeFixture(project, name, service string) api.InstanceFull {
	instance := incustest.Fixture().Instances[0]
	instance.Name, instance.Project = name, project
	instance.ExpandedConfig = map[string]string{"user.label.incus-compose.service": service}

	return instance
}
