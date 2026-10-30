package tui

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// The screen tests draw pkg/tui's View over incustest's fixture daemon and
// compare it with testdata/screens, as pkg/gui's do with its own.

type screenCase struct {
	golden        string
	width, height int
	opts          options
	keys          []string
	open          func(*Model)
}

func TestScreens(t *testing.T) {
	root := t.TempDir()
	local := testStack(t, root, "default", "web", "db")
	other := testStack(t, root, "shop", "api")
	elsewhere := t.TempDir()
	pinned := testStack(t, elsewhere, "shop", "api")
	pinned.Remote = "pve01"

	cases := append(make([]screenCase, 0, 25), []screenCase{
		{golden: "normal-140x40", width: 140, height: 40},
		{golden: "half-140x40", width: 140, height: 40, keys: []string{"+"}},
		{golden: "full-140x40", width: 140, height: 40, keys: []string{"+", "+"}},
		{golden: "portrait-84x50", width: 84, height: 50},
		{golden: "expanded-140x40", width: 140, height: 40, keys: []string{"="}},
		{
			golden: "all-snapshots-140x40", width: 140, height: 40,
			opts: options{configure: func(c *config.UserConfig) { c.Gui.ShowAllSnapshots = true }},
		},
		{golden: "resources-volumes-140x40", width: 140, height: 40, keys: []string{"3", "3"}},
		{golden: "network-leases-140x40", width: 140, height: 40, open: onList("networks")},
		{golden: "network-state-140x40", width: 140, height: 40, open: onList("networks", "]")},
		{golden: "network-acls-140x40", width: 140, height: 40, open: onList("networks", "]", "]")},
		{golden: "network-forwards-140x40", width: 140, height: 40, open: onList("networks", "]", "]", "]")},
		{golden: "profiles-140x40", width: 140, height: 40, open: onList("profiles")},
		{golden: "prune-images-140x40", width: 140, height: 40, keys: []string{"3", "D"}},
		{golden: "menu-90x40", width: 90, height: 40, keys: []string{"x"}},
		{
			golden: "confirmation-140x40", width: 140, height: 40,
			open: func(m *Model) { m.openConfirmation("Confirm", "Are you sure you want to stop web?", nil) },
		},
		{
			golden: "confirmation-wrapped-140x40", width: 140, height: 40,
			open: func(m *Model) {
				words := strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " " + strings.Repeat("c", 40)
				m.openConfirmation("Confirm", words, nil)
			},
		},
		{golden: "error-140x40", width: 140, height: 40, open: func(m *Model) { m.openError("instance is running") }},
		{
			golden: "stacks-140x40", width: 140, height: 40,
			opts: options{
				compose: true, localStack: local, savedStacks: []*commands.ComposeStack{other},
				home: root, server: labelledFixture(map[string]string{"web": "web"}),
			},
		},
		{
			golden: "stacks-elsewhere-140x40", width: 140, height: 40,
			opts: options{compose: true, savedStacks: []*commands.ComposeStack{pinned}, home: elsewhere, remotes: pve01()},
		},
		{
			golden: "stacks-elsewhere-focused-140x40", width: 140, height: 40, keys: []string{"1"},
			opts: options{compose: true, savedStacks: []*commands.ComposeStack{pinned}, home: elsewhere, remotes: pve01()},
		},
	}...)

	for _, border := range []string{"rounded", "single", "double", "hidden"} {
		cases = append(cases, screenCase{
			golden: "border-" + border + "-100x30", width: 100, height: 30,
			opts: options{configure: func(c *config.UserConfig) { c.Gui.Border = border }},
		})
	}

	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			if tc.opts.server == nil {
				tc.opts.fixture = true
			}

			m := start(t, tc.opts, tc.width, tc.height)
			if tc.opts.compose {
				untilRemotes(t, m, func() bool {
					return !slices.ContainsFunc(m.stacks, func(stack *commands.ComposeStack) bool { return stack.StatusPending })
				})
			}
			press(m, tc.keys...)
			if tc.open != nil {
				tc.open(m)
			}

			assertGolden(t, tc.golden, goldenScreen(m))
		})
	}
}

// The toggle leaves the focus on Instances; turning it off again is the
// normal screen.
func TestScreenAllSnapshots(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	for _, golden := range []string{"all-snapshots-140x40", "normal-140x40"} {
		m.toggleAllSnapshots()
		_, cmd := m.Update(nil)
		run(m, cmd)
		assertGolden(t, golden, goldenScreen(m))
	}
}

// onList focuses a list the way pkg/gui's screen tests do, without passing
// the lists before it, then presses keys.
func onList(view string, keys ...string) func(*Model) {
	return func(m *Model) {
		m.focusView(view)
		_, cmd := m.Update(nil)
		run(m, cmd)
		press(m, keys...)
	}
}

// labelledFixture is the fixture daemon with each instance in services
// labelled as incus-compose's service of that name.
func labelledFixture(services map[string]string) *incustest.Server {
	server := incustest.Fixture()
	for i := range server.Instances {
		if service, ok := services[server.Instances[i].Name]; ok {
			server.Instances[i].ExpandedConfig = maps.Clone(server.Instances[i].ExpandedConfig)
			if server.Instances[i].ExpandedConfig == nil {
				server.Instances[i].ExpandedConfig = map[string]string{}
			}
			server.Instances[i].ExpandedConfig["user.label.incus-compose.service"] = service
		}
	}

	return server
}

// pve01 is a second daemon holding the stack "shop" pinned to it.
func pve01() map[string]*incustest.Server {
	return map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{
		Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")},
	})}
}

// The arrows step through every list, the resources' included, and wrap.
func TestArrowsStepThroughEveryList(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	for _, want := range []string{"snapshots", "images", "volumes", "networks", "profiles", "instances"} {
		press(m, "right")
		assert.Equal(t, want, m.currentView())
	}

	press(m, "left")
	assert.Equal(t, "profiles", m.currentView())
	press(m, "left")
	assert.Equal(t, "networks", m.currentView())
	assert.Contains(t, strings.Join(screen(m), "\n"), "Leases")
}

// y offers what an instance has to copy, and nothing it hasn't.
func TestCopyMenuLeavesOutWhatTheInstanceHasNot(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "j", "y")
	text := strings.Join(screen(m), "\n")
	assert.Contains(t, text, "╭─Copy")
	assert.Regexp(t, `│name +web `, text)
	assert.Regexp(t, `│IPv4 +192\.0\.2\.10 `, text)
	assert.NotContains(t, text, "IPv6", "web has no IPv6 address")
	assert.NotContains(t, text, "all addresses", "nor more than one")
}

// A prompt names the item's project only when its list holds several
// projects', and what the daemon would refuse is said without asking.
func TestDeletePromptsNameTheProjectOnlyWhenListsSpanSeveral(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	shows := func(spans bool, open func() tea.Cmd, want string) {
		t.Helper()

		m.showProject, m.imagesSpan, m.volumesSpan, m.networksSpan, m.profilesSpan = spans, spans, spans, spans, spans
		open()
		require.Len(t, m.popups, 1, want)
		assert.Contains(t, ansi.Strip(m.popups[0].message), want)
		press(m, "esc")
	}

	profile := func(name string, usedBy ...string) *commands.Profile {
		return &commands.Profile{Name: name, Profile: api.Profile{Name: name, Project: "stack", UsedBy: usedBy}}
	}
	network := &commands.Network{Name: "br0", Network: api.Network{Name: "br0", Project: "stack", Managed: true}}
	busy := &commands.Network{Name: "br1", Network: api.Network{
		Name: "br1", Project: "stack", Managed: true, UsedBy: []string{"/1.0/profiles/default"},
	}}
	volume := &commands.Volume{Pool: "default", Name: "data", Volume: api.StorageVolume{
		Name: "data", Type: "custom", Project: "stack", UsedBy: []string{"/1.0/instances/db?project=stack"},
	}}
	image := &commands.Image{Fingerprint: "0123456789abcdef", Image: api.Image{
		Project: "stack", Aliases: []api.ImageAlias{{Name: "nginx:alpine"}},
	}}
	instance := &commands.Instance{Name: "web", Project: "stack"}

	shows(false, func() tea.Cmd { return m.profileDelete(profile("web-only")) }, "delete profile web-only?")
	shows(true, func() tea.Cmd { return m.profileDelete(profile("web-only")) }, "delete profile web-only in project stack?")
	shows(true, func() tea.Cmd { return m.profileDelete(profile("default")) }, "default profile can't be deleted")
	shows(false, func() tea.Cmd { return m.profileDelete(profile("web-only", "/1.0/instances/web")) },
		"Profile web-only is still in use (used by: 1)")

	shows(false, func() tea.Cmd { return m.networkDelete(network) }, "delete network br0?")
	shows(true, func() tea.Cmd { return m.networkDelete(busy) }, "Network br1 in project stack is still in use")
	shows(false, func() tea.Cmd { return m.volumeDelete(volume) }, "Volume data is still in use")

	shows(true, func() tea.Cmd { return m.imageDelete(image) }, "delete image nginx:alpine in project stack?")
	shows(true, func() tea.Cmd { return m.instanceDelete(instance) }, "delete instance web in project stack?")
	shows(false, func() tea.Cmd { return m.instanceStop(instance) }, "stop instance web?")
}
