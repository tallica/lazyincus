package tui

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
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
