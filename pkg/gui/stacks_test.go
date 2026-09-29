package gui

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
)

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
func withStacks(t *testing.T, local *commands.ComposeStack, saved ...*commands.ComposeStack) func(*screen) {
	t.Helper()

	return func(s *screen) {
		s.gui.State.ComposeAvailable = true

		byDir := map[string]*commands.ComposeStack{}

		if local != nil {
			s.gui.localStackDir = local.Dir
			byDir[local.Dir] = local
		}

		for _, stack := range saved {
			require.NoError(t, s.gui.Config.AddStack(stack.Dir))
			byDir[stack.Dir] = stack
		}

		s.gui.loadStack = func(dir string) *commands.ComposeStack {
			if stack, ok := byDir[dir]; ok {
				loaded := *stack
				return &loaded
			}

			return &commands.ComposeStack{Dir: dir, Err: errors.New("no compose.yaml found")}
		}
	}
}

// labelled sets the server's instances' compose labels, by name.
func labelled(s *screen, services map[string]string) {
	instances := fixtureServer().Instances
	for i := range instances {
		if service, ok := services[instances[i].Name]; ok {
			instances[i].ExpandedConfig = maps.Clone(instances[i].ExpandedConfig)
			instances[i].ExpandedConfig["user.label.incus-compose.service"] = service
		}
	}

	s.server.SetInstances(instances)
}

// onLoop is a value read on the main loop.
func onLoop[T any](t *testing.T, s *screen, read func() T) T {
	t.Helper()

	var value T
	s.do(t, func() error {
		value = read()
		return nil
	})

	return value
}

func serviceNames(t *testing.T, s *screen) []string {
	t.Helper()

	return onLoop(t, s, func() []string {
		return lo.Map(s.gui.Panels.Services.List.GetAllItems(), func(row *commands.ServiceRow, _ int) string {
			return row.Service.Project + "/" + row.Key()
		})
	})
}

func TestScreenStacks(t *testing.T) {
	root := t.TempDir()
	local := testStack(t, root, "default", "web", "db")
	other := testStack(t, root, "shop", "api")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, local, other)(s)
		labelled(s, map[string]string{"web": "web"})
		// The directories are temporary, so the paths are shown from home.
		s.gui.home = root
	})

	screen := s.settle(t, "Services (default)")
	assert.Contains(t, screen, "[1]─Stacks")
	assert.Contains(t, screen, "[3]─Standalone Instances")

	require.Eventually(t, func() bool {
		return strings.Contains(s.snapshot(t), "│db ")
	}, 5*time.Second, 20*time.Millisecond)

	// The rule runs to the main panel's edge once a layout has measured it.
	assertGolden(t, "stacks-140x40", s.settle(t, "── Drift "+strings.Repeat("─", 40)))
}

// Without incus-compose, the screen goldens are the proof: none has either
// panel.
func TestStacksAreFocusedFirst(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, withStacks(t, testStack(t, t.TempDir(), "default", "web")))

	s.settle(t, "Services (default)")
	assert.Equal(t, "stacks", onLoop(t, s, func() string { return s.gui.currentViewName() }))
}

// The services panel follows the Stacks panel's selection, and a fetch
// still out for the stack it left can't bring that one's rows back.
func TestServicesFollowTheSelectedStack(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	other := testStack(t, t.TempDir(), "shop", "api")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, local, other)(s)
		labelled(s, map[string]string{"web": "web"})
	})

	s.settle(t, "Services (default)")
	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"default/web"})
	}, 5*time.Second, 20*time.Millisecond)

	// Fetched for default, then overtaken by the move to shop.
	late, err := s.gui.fetchServices()
	require.NoError(t, err)

	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "Services (shop)")

	s.do(t, late)
	assert.NotContains(t, serviceNames(t, s), "default/web")

	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"shop/api"})
	}, 5*time.Second, 20*time.Millisecond)
}

// Every listed stack's compose instances are the services panel's; one of
// a project no stack lists, or of a service its stack no longer declares,
// stays standalone.
func TestStandaloneInstancesLeaveOutEveryStack(t *testing.T) {
	instances := fixtureServer().Instances
	compose := func(project, name, service string) api.InstanceFull {
		instance := instances[0]
		instance.Name, instance.Project = name, project
		instance.ExpandedConfig = map[string]string{"user.label.incus-compose.service": service}

		return instance
	}

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, testStack(t, t.TempDir(), "default", "web"), testStack(t, t.TempDir(), "shop", "api"))(s)
		s.server.SetInstances(append(instances[:0:0],
			compose("default", "web", "web"),
			compose("shop", "api-1", "api"),
			compose("shop", "worker-1", "worker"),
			compose("elsewhere", "job-1", "job"),
			instances[1],
		))
	})

	require.Eventually(t, func() bool {
		names := onLoop(t, s, func() []string {
			return lo.Map(s.gui.Panels.Instances.List.GetItems(), func(instance *commands.Instance, _ int) string {
				return instance.Name
			})
		})

		slices.Sort(names)

		return slices.Equal(names, []string{"db", "job-1", "worker-1"})
	}, 5*time.Second, 20*time.Millisecond)

	assert.Equal(t, s.gui.Tr.StandaloneInstancesTitle, onLoop(t, s, func() string { return s.gui.Views.Instances.Title }))
}

func TestAddingAStack(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	notCompose := t.TempDir()
	file := filepath.Join(notCompose, "notes.txt")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil)(s)

		load := s.gui.loadStack
		s.gui.loadStack = func(dir string) *commands.ComposeStack {
			if dir == shop.Dir {
				loaded := *shop
				return &loaded
			}

			return load(dir)
		}
	})
	s.settle(t, s.gui.Tr.NoStacks)

	for input, want := range map[string]string{
		filepath.Join(notCompose, "missing"): "no such file or directory",
		file:                                 "not a directory",
		notCompose:                           "isn't a compose project",
	} {
		assert.ErrorContains(t, s.gui.addStack(input), want, input)
	}

	// Relative to the working directory, and cleaned.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(cwd, shop.Dir)
	require.NoError(t, err)

	require.NoError(t, s.gui.addStack(relative+"/."))
	assert.ErrorContains(t, s.gui.addStack(shop.Dir), "already listed")

	state, err := s.gui.Config.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{shop.Dir}, state.Stacks)

	s.settle(t, "Services (shop)")
}

// The local stack isn't saved, so there's nothing for `D` to remove.
func TestTheLocalStackCannotBeRemoved(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, withStacks(t, testStack(t, t.TempDir(), "default", "web")))
	s.settle(t, "Services (default)")

	s.do(t, func() error {
		stack, err := s.gui.Panels.Stacks.GetSelectedItem()
		if err != nil {
			return err
		}

		return s.gui.stackRemove(stack)
	})

	// The message holds a temp dir path, so where it wraps varies by OS.
	s.settle(t, "─"+s.gui.Tr.ErrorTitle)
	s.do(t, func() error {
		assert.Contains(t, s.gui.Views.Confirmation.Buffer(), "It isn't saved")
		return nil
	})
}

func composeInstance(name, service, status string, devices map[string]map[string]string) *commands.Instance {
	return &commands.Instance{Name: name, Instance: api.InstanceFull{Instance: api.Instance{
		Name:            name,
		Status:          status,
		ExpandedConfig:  map[string]string{"user.label.incus-compose.service": service},
		ExpandedDevices: devices,
	}}}
}

// Drift is an orphan the file has dropped, and a declared service with
// nothing behind it; a service that matches has none.
func TestStackDrift(t *testing.T) {
	state := &stackInstances{
		services: []*commands.ComposeService{
			{Name: "web", Instances: []*commands.Instance{composeInstance("web", "web", "Running", nil)}},
			{Name: "cache"},
		},
		orphans: []*commands.Instance{
			composeInstance("worker-1", "worker", "Running", nil),
			composeInstance("worker-2", "worker", "Stopped", nil),
		},
	}

	assert.Equal(t,
		"cache:        declared, not created\n"+
			"worker:       2 instances, partial, not in the compose file\n",
		stackDriftStr(state))

	assert.Empty(t, stackDriftStr(&stackInstances{services: state.services[:1]}))
}

// A replica goes by its own name, a lone instance by its service's, and one
// with neither an address nor a port is left out.
func TestStackEndpoints(t *testing.T) {
	proxy := map[string]map[string]string{
		"proxy-80": {"type": "proxy", "listen": "tcp:0.0.0.0:8080", "connect": "tcp:127.0.0.1:80"},
	}

	services := []*commands.ComposeService{
		{Name: "web", Instances: []*commands.Instance{
			composeInstance("web-2", "web", "Running", proxy),
			composeInstance("web-1", "web", "Running", proxy),
		}},
		{Name: "db", Instances: []*commands.Instance{composeInstance("db", "db", "Stopped", nil)}},
		{Name: "api", Instances: []*commands.Instance{composeInstance("api", "api", "Running", proxy)}},
	}

	assert.Equal(t,
		"api:          *:8080 → 80\n"+
			"web-1:        *:8080 → 80\n"+
			"web-2:        *:8080 → 80\n",
		stackEndpointsStr(services))
}

// A stack's Logs tab stacks each instance's console log under its service's
// name.
func TestStackLogsStackEachInstance(t *testing.T) {
	root := t.TempDir()

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, root, "default", "web", "db"))(s)
		labelled(s, map[string]string{"web": "web"})
		s.server.ConsoleLogs = map[string]string{"web": "hello from web\n"}
	})

	s.settle(t, "Services (default)")
	require.Eventually(t, func() bool {
		return slices.Contains(serviceNames(t, s), "default/web")
	}, 5*time.Second, 20*time.Millisecond)

	s.do(t, func() error { return s.gui.Panels.Stacks.SetMainTab("logs") })

	screen := s.settle(t, "hello from web")
	assert.Contains(t, screen, "── web ──")
}

// `m` enters the Logs tab with the list as the main panel's parent, so `[`
// and `]` still switch its tabs from there, as does clicking one.
func TestTabsSwitchAfterJumpingToLogs(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)
		labelled(s, map[string]string{"web": "web"})
	})

	s.settle(t, "Services (default)")
	require.Eventually(t, func() bool {
		return slices.Contains(serviceNames(t, s), "default/web")
	}, 5*time.Second, 20*time.Millisecond)

	tab := func() string {
		return onLoop(t, s, func() string {
			return s.gui.currentViewName() + " " + s.gui.Views.Main.Tabs[s.gui.Views.Main.TabIndex]
		})
	}

	for _, panel := range []rune{'1', '2'} {
		s.press(t, panel)
		s.press(t, 'm')
		require.Eventually(t, func() bool { return tab() == "main Logs" }, 5*time.Second, 20*time.Millisecond)

		s.press(t, '[')
		require.Eventually(t, func() bool { return tab() == "main Info" }, 5*time.Second, 20*time.Millisecond,
			"panel %c: %s", panel, tab())

		titles := strings.Split(s.snapshot(t), "\n")[0]
		s.click(t, utf8.RuneCountInString(titles[:strings.Index(titles, "Config")]), 0)
		require.Eventually(t, func() bool { return tab() == "main Config" }, 5*time.Second, 20*time.Millisecond,
			"panel %c: %s", panel, tab())
	}
}
