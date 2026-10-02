package gui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

var updateGolden = flag.Bool("update", false, "rewrite the screen tests' golden files")

// Dates render in local time; the golden screens were taken in UTC.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

// The screen tests draw the real app on a headless gocui over incustest's
// daemon and compare what's on screen with testdata/screens. They're what
// catches a layout that shifts under a gocui upgrade.

func fixtureServer() *incustest.Server {
	running := func(project, name, ipv4 string, snapshots int) api.InstanceFull {
		instance := api.InstanceFull{
			Instance: api.Instance{
				Name: name, Project: project, Status: "Running", Type: "container",
				InstancePut: api.InstancePut{
					Architecture: "x86_64", Profiles: []string{"default"},
					Config: map[string]string{"volatile.base_image": "0123456789abcdef0123456789abcdef"},
				},
				ExpandedDevices: map[string]map[string]string{"eth0": {"type": "nic", "network": "incusbr0"}},
				ExpandedConfig: map[string]string{
					"image.description": "Alpine 3.22 amd64",
				},
			},
			State: &api.InstanceState{
				Status:    "Running",
				Processes: 12,
				Network: map[string]api.InstanceStateNetwork{
					"eth0": {HostName: "veth" + ipv4[len("192.0.2."):], Addresses: []api.InstanceStateNetworkAddress{
						{Family: "inet", Address: ipv4, Scope: "global"},
					}},
				},
			},
		}

		for i := range snapshots {
			instance.Snapshots = append(instance.Snapshots, api.InstanceSnapshot{
				Name:      name + "/daily-" + string(rune('a'+i)),
				CreatedAt: time.Date(2026, 9, 20+i, 6, 0, 0, 0, time.UTC),
			})
		}

		return instance
	}

	web := running("default", "web", "192.0.2.10", 2)
	web.ExpandedDevices["eth0"]["security.acls"] = "web-only"

	stopped := api.InstanceFull{Instance: api.Instance{
		Name: "db", Project: "default", Status: "Stopped", Type: "container",
		InstancePut: api.InstancePut{Architecture: "x86_64"},
	}}

	return incustest.New(incustest.Server{
		Version: "7.4",
		Instances: []api.InstanceFull{
			web,
			stopped,
			running("default", "a-name-long-enough-to-be-cut-off", "192.0.2.144", 0),
		},
		Images: []api.Image{
			{
				Fingerprint: "0123456789abcdef0123456789abcdef", Project: "default", Type: "container",
				Properties: map[string]string{"description": "Alpine 3.22 amd64"},
				Size:       3 << 20, LastUsedAt: time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC),
			},
			{
				Fingerprint: "fedcba9876543210fedcba9876543210", Project: "default", Type: "container",
				Properties: map[string]string{"description": "Debian 13 amd64"},
				Size:       90 << 20, Cached: true,
			},
		},
		Networks: []api.Network{
			{Name: "incusbr0", Type: "bridge", Managed: true, Project: "default", NetworkPut: api.NetworkPut{
				Config: map[string]string{"security.acls": "isolate", "security.acls.default.egress.action": "allow"},
			}},
			{Name: "eth0", Type: "physical", Project: "default"},
		},
		Profiles: []api.Profile{{
			Name: "default", Project: "default", UsedBy: []string{"/1.0/instances/web", "/1.0/instances/db"},
			ProfilePut: api.ProfilePut{
				Description: "Default Incus profile",
				Devices: map[string]map[string]string{
					"eth0": {"type": "nic", "network": "incusbr0", "name": "eth0"},
					"root": {"type": "disk", "pool": "default", "path": "/"},
				},
			},
		}},
		NetworkLeases: map[string]map[string][]api.NetworkLease{
			"incusbr0": {"default": {
				{Hostname: "incusbr0.gw", Address: "192.0.2.1", Type: "GATEWAY"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "192.0.2.10", Type: "DYNAMIC"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "2001:db8::10", Type: "DYNAMIC"},
			}},
		},
		NetworkACLs: map[string]api.NetworkACL{
			"isolate": {NetworkACLPost: api.NetworkACLPost{Name: "isolate"}, NetworkACLPut: api.NetworkACLPut{
				Description: "Keep the stack to itself",
				Ingress: []api.NetworkACLRule{
					{Action: "reject", Source: "10.0.0.0/8", State: "enabled"},
					{Action: "allow", Protocol: "tcp", DestinationPort: "80,443", State: "logged"},
				},
				Egress: []api.NetworkACLRule{{Action: "drop", Destination: "192.0.2.53", Protocol: "udp", DestinationPort: "53", State: "disabled"}},
			}},
			"web-only": {NetworkACLPost: api.NetworkACLPost{Name: "web-only"}, NetworkACLPut: api.NetworkACLPut{
				Ingress: []api.NetworkACLRule{{Action: "allow", Protocol: "tcp", DestinationPort: "80", State: "enabled"}},
			}},
		},
		NetworkForwards: map[string][]api.NetworkForward{
			"incusbr0": {{ListenAddress: "198.51.100.7", NetworkForwardPut: api.NetworkForwardPut{
				Ports: []api.NetworkForwardPort{
					{Protocol: "tcp", ListenPort: "443", TargetAddress: "192.0.2.10", Description: "https"},
				},
			}}},
		},
		NetworkStates: map[string]api.NetworkState{
			"incusbr0": {
				State: "up", Type: "broadcast", Hwaddr: "10:66:6a:00:00:01", Mtu: 1500,
				Addresses: []api.NetworkStateAddress{
					{Family: "inet", Address: "192.0.2.1", Netmask: "24", Scope: "global"},
				},
				Counters: &api.NetworkStateCounters{BytesReceived: 2048, PacketsReceived: 20, BytesSent: 1024, PacketsSent: 10},
				Bridge:   &api.NetworkStateBridge{ID: "8000.10666a000001", UpperDevices: []string{"veth10", "veth99"}},
			},
		},
		Volumes: map[string][]api.StorageVolume{
			"default": {
				{Name: "data", Type: "custom", Project: "default"},
				{Name: "web", Type: "container", Project: "default", UsedBy: []string{"/1.0/instances/web"}},
			},
		},
		PoolSpace:   map[string]api.ResourcesStoragePoolSpace{"default": {Used: 5 << 30, Total: 50 << 30}},
		VolumeUsage: map[string]uint64{"data": 512 << 20},
		VolumeSnapshots: map[string][]api.StorageVolumeSnapshot{"data": {{
			Name: "before-migration", CreatedAt: time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC),
		}}},
	})
}

type screen struct {
	gui    *Gui
	g      *gocui.Gui
	server *incustest.Server
	done   chan error
}

// startScreen runs the app on a width×height headless terminal until the
// test ends.
func startScreen(t *testing.T, width, height int, configure func(*config.UserConfig)) *screen {
	t.Helper()

	return startScreenWith(t, width, height, configure, nil)
}

// startScreenWith is startScreen with prepare run before the app starts,
// for what Run would set up and run doesn't: a compose stack.
func startScreenWith(t *testing.T, width, height int, configure func(*config.UserConfig), prepare func(*screen)) *screen {
	t.Helper()

	userConfig := config.GetDefaultConfig()
	if configure != nil {
		configure(&userConfig)
	}

	appConfig := &config.AppConfig{
		Name:       "lazyincus",
		Version:    "test",
		UserConfig: &userConfig,
		ConfigDir:  t.TempDir(),
	}

	log := commands.NewDummyLog()
	tr := i18n.NewTranslationSet(log, "en")
	osCommand := commands.NewOSCommand(log, appConfig)
	server := fixtureServer()
	incusCommand := commands.NewIncusCommandWithClient(log, osCommand, tr, appConfig, server, "fake")

	gui, err := NewGui(log, incusCommand, osCommand, tr, appConfig)
	require.NoError(t, err)

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
		Headless:         true,
		Width:            width,
		Height:           height,
		// Its event poller reads a channel rather than gocui's global
		// Screen, which the next test's gocui replaces while the last one's
		// poller would still be reading it.
		PlayRecording: true,
	})
	require.NoError(t, err)

	s := &screen{gui: gui, g: g, server: server, done: make(chan error, 1)}

	if prepare != nil {
		prepare(s)
	}

	go func() { s.done <- gui.run(g) }()

	t.Cleanup(func() {
		g.Update(func(*gocui.Gui) error { return gocui.ErrQuit })

		select {
		case err := <-s.done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("the app didn't quit")
		}
	})

	return s
}

// do runs f on the main loop, the way a keypress would.
func (s *screen) do(t *testing.T, f func() error) {
	t.Helper()

	result := make(chan error, 1)
	s.g.Update(func(*gocui.Gui) error {
		result <- f()
		return nil
	})

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the main loop didn't run the update")
	}
}

// press types a key through gocui's own dispatch, keybindings and all,
// rather than calling a handler.
func (s *screen) press(t *testing.T, key rune) {
	t.Helper()

	select {
	case s.g.ReplayedEvents.Keys <- &gocui.TcellKeyEventWrapper{Key: tcell.KeyRune, Ch: key}:
	case <-time.After(5 * time.Second):
		t.Fatalf("the main loop didn't take %q", key)
	}
}

// click presses and releases the left button at a cell of the screen.
func (s *screen) click(t *testing.T, x, y int) {
	t.Helper()

	for _, buttons := range []tcell.ButtonMask{tcell.Button1, tcell.ButtonNone} {
		select {
		case s.g.ReplayedEvents.MouseEvents <- &gocui.TcellMouseEventWrapper{X: x, Y: y, ButtonMask: buttons}:
		case <-time.After(5 * time.Second):
			t.Fatalf("the main loop didn't take a click at %d,%d", x, y)
		}
	}
}

// ready waits for startup's fetches to land and the screen to settle. The
// resources are drawn behind tabs, so it's their panels rather than the
// screen that say they've arrived.
func (s *screen) ready(t *testing.T) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		loaded := false
		s.do(t, func() error {
			loaded = s.gui.Panels.Images.List.Len() > 0 && s.gui.Panels.Networks.List.Len() > 0 &&
				s.gui.Panels.Profiles.List.Len() > 0
			return nil
		})

		if loaded {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the networks never arrived")
		}

		time.Sleep(20 * time.Millisecond)
	}

	return s.settle(t, "Alpine 3.22 amd64")
}

// settle waits until the screen shows want and has stopped changing, and
// returns it with trailing blanks trimmed off each line.
func (s *screen) settle(t *testing.T, want string) string {
	t.Helper()

	var last string
	stable := 0
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		current := s.snapshot(t)

		if current == last && strings.Contains(current, want) {
			stable++
			if stable == 3 {
				return current
			}
		} else {
			stable = 0
		}

		last = current
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("screen never settled showing %q:\n%s", want, last)

	return ""
}

func (s *screen) snapshot(t *testing.T) string {
	t.Helper()

	result := make(chan string, 1)
	s.g.Update(func(g *gocui.Gui) error {
		result <- g.Snapshot()
		return nil
	})

	select {
	case snapshot := <-result:
		lines := strings.Split(snapshot, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " ")
		}

		return strings.Join(lines, "\n")
	case <-time.After(5 * time.Second):
		t.Fatal("the main loop didn't take a snapshot")
		return ""
	}
}

// assertGolden compares a screen with testdata/screens/<name>.txt, or
// rewrites that file under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", "screens", name+".txt")

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
		return
	}

	want, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no golden screen at %s - run the test with -update to write it:\n%s", path, got)
	}
	require.NoError(t, err)

	assert.Equal(t, string(want), got, "screen differs from %s", path)
}

func TestScreenNormal(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	assertGolden(t, "normal-140x40", s.ready(t))
}

func TestScreenHalf(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, s.gui.nextScreenMode)
	assertGolden(t, "half-140x40", s.settle(t, "192.0.2.10"))
}

func TestScreenFull(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, s.gui.nextScreenMode)
	s.do(t, s.gui.nextScreenMode)
	assertGolden(t, "full-140x40", s.settle(t, "192.0.2.10"))
}

func TestScreenPortrait(t *testing.T) {
	s := startScreen(t, 84, 50, nil)
	assertGolden(t, "portrait-84x50", s.ready(t))
}

func TestScreenExpandedSidePanel(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, s.gui.toggleExpandSidePanel)
	assertGolden(t, "expanded-140x40", s.ready(t))
}

func TestScreenAllSnapshots(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, func() error { return s.gui.handleToggleAllSnapshots(s.g, s.gui.Views.Snapshots) })
	assertGolden(t, "all-snapshots-140x40", s.settle(t, "Snapshots (all)"))

	s.do(t, func() error { return s.gui.handleToggleAllSnapshots(s.g, s.gui.Views.Snapshots) })
	assertGolden(t, "normal-140x40", s.settle(t, "Snapshots (a-name"))
}

func TestScreenAllSnapshotsFromConfig(t *testing.T) {
	s := startScreen(t, 140, 40, func(userConfig *config.UserConfig) {
		userConfig.Gui.ShowAllSnapshots = true
	})
	assertGolden(t, "all-snapshots-140x40", s.settle(t, "daily-b"))
}

func TestScreenResourcesTabs(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	pressThree := func() {
		s.do(t, func() error { return s.gui.handleGoToWindow(resourcesWindow)(s.g, nil) })
	}

	pressThree()
	s.do(t, s.gui.cycleWindowTab(1))
	assertGolden(t, "resources-volumes-140x40", s.settle(t, "Pool:"))

	// The number key again moves on to the next list.
	pressThree()
	s.settle(t, "incusbr0")

	// Leaving and coming back lands on the list last used.
	s.do(t, s.gui.cycleSidePanel(1))
	s.settle(t, "Name:         a-name")
	pressThree()
	s.settle(t, "incusbr0")
}

// Networks hold the host's interfaces back until asked for, and lead with
// who has which address.
func TestScreenNetworks(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Networks) })
	screen := s.settle(t, "2001:db8::10")
	assert.NotContains(t, screen, "│eth0")
	assertGolden(t, "network-leases-140x40", screen)

	s.do(t, s.gui.Panels.Networks.HandleNextMainTab)
	assertGolden(t, "network-state-140x40", s.settle(t, "Ports:"))

	s.do(t, func() error { return s.gui.handleToggleUnmanagedNetworks(s.g, nil) })
	s.settle(t, "│eth0")
}

func TestScreenPruneImages(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Images) })
	s.do(t, func() error { return s.gui.handlePruneImages(s.g, s.gui.Views.Images) })
	assertGolden(t, "prune-images-140x40", s.settle(t, "every unused image (1, 90.00MiB)"))

	s.do(t, s.gui.Panels.Menu.HandleNextLine)
	s.do(t, s.gui.Panels.Menu.HandleClick)
	screen := s.settle(t, "Delete these 1 images")
	assert.Contains(t, screen, "Debian 13 amd64 fedcba987654")
}

// Arrows step through every list, the resources' included, and wrap.
func TestArrowsStepThroughEveryList(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	current := func() string {
		name := ""
		s.do(t, func() error { name = s.gui.currentViewName(); return nil })
		return name
	}

	for _, want := range []string{"snapshots", "images", "volumes", "networks", "profiles", "instances"} {
		s.do(t, s.gui.cycleSideView(1))
		assert.Equal(t, want, current())
	}

	s.do(t, s.gui.cycleSideView(-1))
	assert.Equal(t, "profiles", current())
	s.do(t, s.gui.cycleSideView(-1))
	assert.Equal(t, "networks", current())

	// The resources panel shows whichever list the arrows reached.
	assert.Contains(t, s.settle(t, "Leases"), "│incusbr0")
}

// u narrows the instances to a resource's users, and esc brings them all
// back.
func TestShowUsers(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Networks) })
	s.do(t, func() error { return s.gui.showNetworkUsers(s.gui.Panels.Networks.List.GetItems()[0]) })

	screen := s.settle(t, "Instances using incusbr0")
	assert.Contains(t, screen, "│web ")
	assert.NotContains(t, screen, "│db ")

	s.do(t, s.gui.escape)
	screen = s.settle(t, "│db ")
	assert.NotContains(t, screen, "Instances using")

	// Back on the network it was asked of.
	s.do(t, func() error {
		assert.Equal(t, "networks", s.gui.currentViewName())
		return nil
	})
}

func TestScreenNetworkACLs(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Networks) })
	s.do(t, func() error { return s.gui.Panels.Networks.SetMainTab("acls") })

	assertGolden(t, "network-acls-140x40", s.settle(t, "Egress:"))
}

func TestScreenNetworkForwards(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Networks) })
	s.do(t, func() error { return s.gui.Panels.Networks.SetMainTab("forwards") })

	assertGolden(t, "network-forwards-140x40", s.settle(t, "192.0.2.10:443"))
}

// The snapshots panel follows a custom volume selected in the volumes list.
func TestScreenVolumeSnapshots(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Volumes) })
	screen := s.settle(t, "Snapshots (data)")
	assert.Contains(t, screen, "before-migration")

	s.do(t, s.gui.Panels.Volumes.HandleNextLine)
	screen = s.settle(t, "Name:         web")
	assert.Contains(t, screen, "Snapshots (data)", "an instance's own volume leaves the panel be")

	s.do(t, s.gui.cycleSideView(1))
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Instances) })
	assert.NotContains(t, s.settle(t, "Snapshots (a-name"), "before-migration")
}

func TestScreenProfiles(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Profiles) })
	assertGolden(t, "profiles-140x40", s.settle(t, "network=incusbr0"))

	s.do(t, func() error { return s.gui.showProfileUsers(s.gui.Panels.Profiles.List.GetItems()[0]) })
	screen := s.settle(t, "Instances using default")
	assert.Contains(t, screen, "│web ")
	assert.Contains(t, screen, "│db ")
}

// y offers what an item has to copy, and nothing it hasn't.
func TestCopyMenu(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, s.gui.Panels.Instances.HandleNextLine)
	s.do(t, func() error { return s.gui.instanceCopy(s.gui.Panels.Instances.List.GetItems()[1]) })

	screen := s.settle(t, "╭─Copy")
	assert.Regexp(t, `│name +web `, screen)
	assert.Regexp(t, `│IPv4 +192\.0\.2\.10 `, screen)
	assert.NotContains(t, screen, "IPv6", "web has no IPv6 address")
	assert.NotContains(t, screen, "all addresses", "nor more than one")
}

// A prompt names the item's project only when its list holds several
// projects', and what the daemon would refuse - a profile named default,
// anything still in use - is said without asking.
func TestDeletePrompts(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	shows := func(spans bool, open func() error, want string) {
		t.Helper()

		s.do(t, func() error {
			s.gui.State.SpansProjects = spansProjects{
				Instances: spans, Images: spans, Volumes: spans, Networks: spans, Profiles: spans,
			}
			return open()
		})

		// settle fails the test if the text never appears.
		s.settle(t, want)
		s.do(t, s.gui.closeConfirmationPrompt)
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

	shows(false, func() error { return s.gui.profileDelete(profile("web-only")) }, "delete profile web-only?")
	shows(true, func() error { return s.gui.profileDelete(profile("web-only")) }, "delete profile web-only in project stack?")
	shows(true, func() error { return s.gui.profileDelete(profile("default")) }, "default profile can't be deleted")
	shows(false, func() error { return s.gui.profileDelete(profile("web-only", "/1.0/instances/web")) },
		"Profile web-only is still in use (used by: 1)")

	shows(false, func() error { return s.gui.networkDelete(network) }, "delete network br0?")
	shows(true, func() error { return s.gui.networkDelete(busy) }, "Network br1 in project stack is still in use")
	shows(false, func() error { return s.gui.volumeDelete(volume) }, "Volume data is still in use")

	shows(true, func() error { return s.gui.imageDelete(image) }, "delete image nginx:alpine in project stack?")
	shows(true, func() error { return s.gui.instanceDelete(instance) }, "delete instance web in project stack?")
	shows(false, func() error { return s.gui.instanceStop(instance) }, "stop instance web?")
}

func TestScreenMenu(t *testing.T) {
	s := startScreen(t, 90, 40, nil)
	s.ready(t)
	s.do(t, func() error { return s.gui.handleCreateOptionsMenu(s.g, s.gui.Views.Instances) })
	assertGolden(t, "menu-90x40", s.settle(t, "focus resources panel"))
}

func TestScreenConfirmation(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, func() error {
		return s.gui.createConfirmationPanel("Confirm", "Are you sure you want to stop web?", nil, nil)
	})
	assertGolden(t, "confirmation-140x40", s.settle(t, "stop web?"))
}

// A popup's text wraps at word boundaries, so it can take more rows than its
// length alone says: three 40-column words need three rows at this width,
// where a count of characters gives two.
func TestScreenWordWrappedConfirmation(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	words := strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " " + strings.Repeat("c", 40)
	s.do(t, func() error { return s.gui.createConfirmationPanel("Confirm", words, nil, nil) })

	screen := s.settle(t, strings.Repeat("a", 40))
	assert.Contains(t, screen, strings.Repeat("c", 40))
	assertGolden(t, "confirmation-wrapped-140x40", screen)
}

func TestScreenErrorPopup(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)
	s.do(t, func() error { return s.gui.createErrorPanel("instance is running") })
	assertGolden(t, "error-140x40", s.settle(t, "instance is running"))
}

func TestScreenBorders(t *testing.T) {
	for _, border := range []string{"rounded", "single", "double", "hidden"} {
		t.Run(border, func(t *testing.T) {
			s := startScreen(t, 100, 30, func(c *config.UserConfig) { c.Gui.Border = border })
			assertGolden(t, "border-"+border+"-100x30", s.ready(t))
		})
	}
}
