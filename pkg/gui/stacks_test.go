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
	"github.com/tallica/lazyincus/pkg/commands/incustest"
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
			require.NoError(t, s.gui.Config.AddStack(stack.Ref()))
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
			if instances[i].ExpandedConfig == nil {
				instances[i].ExpandedConfig = map[string]string{}
			}

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

	shows := func(want ...string) func() bool {
		return func() bool {
			names := onLoop(t, s, func() []string {
				return lo.Map(s.gui.Panels.Instances.List.GetItems(), func(instance *commands.Instance, _ int) string {
					return instance.Name
				})
			})

			slices.Sort(names)

			return slices.Equal(names, want)
		}
	}
	title := func() string { return onLoop(t, s, func() string { return s.gui.Views.Instances.Title }) }

	require.Eventually(t, shows("db", "job-1", "worker-1"), 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, s.gui.Tr.StandaloneInstancesTitle, title())

	// C puts the stacks' own back, and takes them out again.
	s.press(t, '3')
	s.press(t, 'C')
	require.Eventually(t, shows("api-1", "db", "job-1", "web", "worker-1"), 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, s.gui.Tr.InstancesTitle, title())

	s.press(t, 'C')
	require.Eventually(t, shows("db", "job-1", "worker-1"), 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, s.gui.Tr.StandaloneInstancesTitle, title())
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
	assert.Equal(t, []string{"fake:" + shop.Dir}, state.Stacks)

	s.settle(t, "Services (shop)")
}

// withRemotes stands each server in for the CLI remote it's keyed by, the
// session's own fixture for "fake", and "down" for a remote that's known
// but doesn't answer.
func withRemotes(servers map[string]*incustest.Server) func(*screen) {
	return func(s *screen) {
		servers = maps.Clone(servers)
		servers["fake"] = s.server

		names := append(slices.Sorted(maps.Keys(servers)), "down")
		slices.Sort(names)

		s.gui.remotes.names = func() []string { return names }
		s.gui.remotes.known = func(remote string) bool { return slices.Contains(names, remote) }
		s.gui.remotes.connect = func(remote string) (*commands.IncusCommand, error) {
			server, ok := servers[remote]
			if !ok {
				return nil, &commands.ConnectError{Remote: remote, Err: errors.New("no answer")}
			}

			command := s.gui.IncusCommand
			return commands.NewIncusCommandWithClient(command.Log, command.OSCommand, command.Tr, command.Config, server, remote), nil
		}
	}
}

func composeFixture(project, name, service string) api.InstanceFull {
	instance := fixtureServer().Instances[0]
	instance.Name, instance.Project = name, project
	instance.ExpandedConfig = map[string]string{"user.label.incus-compose.service": service}

	return instance
}

// A stack pinned to another remote is that remote's: its statuses, its
// services and their instances. The session's own instances of a project
// with the same name stay standalone.
func TestAStackPinnedToARemote(t *testing.T) {
	pve01 := incustest.New(incustest.Server{Instances: []api.InstanceFull{composeFixture("shop", "api-1", "api")}})
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		s.server.SetInstances([]api.InstanceFull{composeFixture("shop", "web", "api")})
	})

	s.settle(t, "Services (shop on pve01)")
	assert.Regexp(t, `│  pve01 shop`, s.snapshot(t))

	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"shop/api"})
	}, 5*time.Second, 20*time.Millisecond)

	instance := onLoop(t, s, func() *commands.Instance {
		return s.gui.Panels.Services.List.GetAllItems()[0].Service.Instances[0]
	})
	assert.Equal(t, "api-1", instance.Name)
	assert.Equal(t, "pve01", instance.Remote)

	// The snapshots panel, pointed at it the way a new snapshot does,
	// says where too.
	s.do(t, func() error { return s.gui.refreshSnapshotsFor(instance.Name, instance.Remote, instance) })
	s.settle(t, "Snapshots (api-1 on pve01)")

	s.do(t, func() error { return s.gui.snapshotCreatePrompt(instance) })
	s.settle(t, "New snapshot of api-1 on pve01")
	s.do(t, s.gui.closeSnapshotPrompt)

	// What would act on it says where.
	assert.Equal(t, "api-1 on pve01", onLoop(t, s, func() string { return s.gui.qualifiedInstance(instance) }))
	s.do(t, func() error {
		service := s.gui.Panels.Services.List.GetAllItems()[0].Service
		return s.gui.composeConfirm(s.gui.Tr.ConfirmComposeStop, serviceTarget(service), "stop")
	})
	s.settle(t, "stop service api on pve01?")

	require.Eventually(t, func() bool {
		return onLoop(t, s, func() bool {
			return slices.ContainsFunc(s.gui.Panels.Instances.List.GetItems(), func(instance *commands.Instance) bool {
				return instance.Name == "web"
			})
		})
	}, 5*time.Second, 20*time.Millisecond)
}

// A remote that doesn't answer marks its own stacks, and only those.
func TestAStackOnAnUnreachableRemote(t *testing.T) {
	unreachable := testStack(t, t.TempDir(), "shop", "api")
	unreachable.Remote = "down"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"), unreachable)(s)
		withRemotes(map[string]*incustest.Server{"pve01": nil})(s)
	})

	s.settle(t, "unreachable")

	// Selected, it says why on its Info tab rather than in a popup, and
	// the services list says where to look.
	s.do(t, s.gui.Panels.Stacks.HandleNextLine)
	s.settle(t, "no answer")
	require.Eventually(t, func() bool {
		return strings.Contains(onLoop(t, s, s.gui.Views.Services.Buffer), "can't reach down")
	}, 5*time.Second, 20*time.Millisecond)
	assert.False(t, onLoop(t, s, func() bool { return s.gui.Views.Confirmation.Visible }))
	assert.Equal(t, []string{"none", "unreachable"}, onLoop(t, s, func() []string {
		return lo.Map(s.gui.Panels.Stacks.List.GetAllItems(), func(stack *commands.ComposeStack, _ int) string {
			if stack.StatusErr != nil {
				return "unreachable"
			}

			return stack.Status()
		})
	}))
}

// `a` takes a remote ahead of the directory, and only a remote it knows.
func TestAddingAStackOnARemote(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil)(s)
		withRemotes(map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})})(s)
		s.gui.loadStack = func(dir string) *commands.ComposeStack {
			loaded := *shop
			loaded.Dir = dir
			return &loaded
		}
	})
	s.settle(t, s.gui.Tr.NoStacks)

	assert.ErrorContains(t, s.gui.addStack("nope:"+shop.Dir), "no such file or directory")
	require.NoError(t, s.gui.addStack("pve01:"+shop.Dir))
	assert.ErrorContains(t, s.gui.addStack("pve01:"+shop.Dir), "already listed")
	// With no remote, it's pinned to the session's.
	require.NoError(t, s.gui.addStack(shop.Dir))
	assert.ErrorContains(t, s.gui.addStack("fake:"+shop.Dir), "already listed")

	state, err := s.gui.Config.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"pve01:" + shop.Dir, "fake:" + shop.Dir}, state.Stacks)

	// Removing it removes what was saved.
	require.Eventually(t, func() bool {
		return onLoop(t, s, func() int { return len(s.gui.Panels.Stacks.List.GetAllItems()) }) == 2
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, s.gui.Config.RemoveStack(onLoop(t, s, func() string {
		return s.gui.Panels.Stacks.List.GetAllItems()[0].Ref()
	})))

	state, err = s.gui.Config.LoadAppState()
	require.NoError(t, err)
	assert.Len(t, state.Stacks, 1)
}

// `e` starts from the stack's own entry, and what's saved takes its place.
func TestEditingAStack(t *testing.T) {
	shop := testStack(t, t.TempDir(), "shop", "api")
	shop.Remote = "fake"
	moved := t.TempDir()

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, shop)(s)
		withRemotes(map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})})(s)
		s.gui.loadStack = func(dir string) *commands.ComposeStack {
			loaded := *shop
			loaded.Dir = dir
			return &loaded
		}
	})
	s.settle(t, "Services (shop)")

	s.do(t, func() error {
		stack, err := s.gui.Panels.Stacks.GetSelectedItem()
		if err != nil {
			return err
		}

		return s.gui.stackEdit(stack)
	})
	s.settle(t, s.gui.Tr.EditStackPrompt)
	assert.Equal(t, "fake:"+shop.Dir, strings.TrimSpace(onLoop(t, s, s.gui.Views.Confirmation.Buffer)))
	s.do(t, s.gui.closeConfirmationPrompt)

	// Unchanged is no change, and a directory that isn't there is refused.
	require.NoError(t, s.gui.saveStack("fake:"+shop.Dir, shop.Ref()))
	assert.ErrorContains(t, s.gui.saveStack("pve01:"+filepath.Join(moved, "missing"), shop.Ref()), "no such file")

	require.NoError(t, s.gui.saveStack("pve01:"+moved, shop.Ref()))

	state, err := s.gui.Config.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"pve01:" + moved}, state.Stacks)
	s.settle(t, "Services (shop on pve01)")
}

// The local stack and the same directory saved for the session's remote
// are one row, both listed, which D takes off the list as saved; saved for
// another remote, it's a row of its own.
func TestTheLocalStackSavedIsOneRow(t *testing.T) {
	local := testStack(t, t.TempDir(), "default", "web")
	saved, elsewhere := *local, *local
	saved.Remote, elsewhere.Remote = "fake", "pve01"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, local, &saved, &elsewhere)(s)
		withRemotes(map[string]*incustest.Server{"pve01": incustest.New(incustest.Server{})})(s)
	})
	s.settle(t, "Services (default)")

	stacks := onLoop(t, s, s.gui.Panels.Stacks.List.GetItems)
	require.Len(t, stacks, 2)
	assert.True(t, stacks[0].Local && stacks[0].Saved)
	assert.Equal(t, "fake:"+local.Dir, stacks[0].Ref())
	assert.Equal(t, "pve01:"+local.Dir, stacks[1].Ref())

	// On pve01 it's the other entry that's the local stack, and the rows
	// stay where they were.
	s.do(t, func() error { return s.gui.switchToRemote("pve01") })
	s.settle(t, "(pve01/all projects)")
	require.Eventually(t, func() bool {
		stacks := onLoop(t, s, s.gui.Panels.Stacks.List.GetItems)
		return len(stacks) == 2 && stacks[1].Local
	}, 5*time.Second, 20*time.Millisecond)

	stacks = onLoop(t, s, s.gui.Panels.Stacks.List.GetItems)
	assert.Equal(t, "fake:"+local.Dir, stacks[0].Ref())
	assert.Equal(t, "pve01:"+local.Dir, stacks[1].Ref())
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
		stackEndpointsStr(services, ""))
}

// A stack's Logs tab stacks each instance's console log under its service's
// name, one blank line before the next heading.
func TestStackLogsStackEachInstance(t *testing.T) {
	root := t.TempDir()

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, root, "default", "web", "db"))(s)
		labelled(s, map[string]string{"web": "web", "db": "db"})
		s.server.ConsoleLogs = map[string]string{"web": "hello from web\n", "db": "hello from db\n"}
	})

	s.settle(t, "Services (default)")
	require.Eventually(t, func() bool {
		return slices.Contains(serviceNames(t, s), "default/web")
	}, 5*time.Second, 20*time.Millisecond)

	s.do(t, func() error { return s.gui.Panels.Stacks.SetMainTab("logs") })

	screen := s.settle(t, "hello from web")
	assert.Contains(t, screen, "── web ──")

	lines := strings.Split(screen, "\n")
	dbLog := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "hello from db") })
	webHeading := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "── web ──") })
	assert.Equal(t, dbLog+2, webHeading, "one blank line between a log and the next heading")
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

// y on a stack offers its project, its directory, and each published port
// at the address it's reached from here.
func TestCopyingFromAStack(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, stack)(s)
		s.server.URL = "https://192.0.2.5:8443"

		instances := fixtureServer().Instances
		for i := range instances {
			if instances[i].Name == "web" {
				instances[i].ExpandedConfig = maps.Clone(instances[i].ExpandedConfig)
				instances[i].ExpandedConfig["user.label.incus-compose.service"] = "web"
				instances[i].ExpandedDevices = map[string]map[string]string{
					"proxy-80": {"type": "proxy", "listen": "tcp:0.0.0.0:8080", "connect": "tcp:127.0.0.1:80"},
				}
			}
		}

		s.server.SetInstances(instances)
	})

	s.settle(t, "192.0.2.5:8080 → 80")
	s.press(t, 'y')

	menu := s.settle(t, "web → 80")
	assert.Contains(t, menu, "192.0.2.5:8080")
	assert.Contains(t, menu, "project")
	assert.Contains(t, menu, "directory")
}

// A volume name too long for the usual column moves every value in Usage
// over with it, so they still line up.
func TestStackUsageLinesUpItsValues(t *testing.T) {
	instance := composeInstance("mosquitto", "mosquitto", "Running", map[string]map[string]string{
		"root":               {"type": "disk", "path": "/", "pool": "default"},
		"vol-mosquitto-data": {"type": "disk", "path": "/data", "pool": "default", "source": "vol-mosquitto-data"},
		"eth0":               {"type": "nic", "network": "ic-hnyq46sd3h"},
	})
	instance.Instance.State = &api.InstanceState{Processes: 3}

	columns := map[int]bool{}

	for _, line := range strings.Split(strings.TrimSpace(stackUsageStr([]*commands.Instance{instance})), "\n") {
		if label, _, ok := strings.Cut(line, ": "); ok && !strings.HasSuffix(line, ":") {
			rest := line[len(label)+2:]
			columns[len(line)-len(strings.TrimLeft(rest, " "))] = true
		}
	}

	assert.Len(t, columns, 1, "every value at the same column")
}
