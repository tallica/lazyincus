package gui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	stopped := api.InstanceFull{Instance: api.Instance{
		Name: "db", Project: "default", Status: "Stopped", Type: "container",
		InstancePut: api.InstancePut{Architecture: "x86_64"},
	}}

	return incustest.New(incustest.Server{
		Instances: []api.InstanceFull{
			running("default", "web", "192.0.2.10", 2),
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
			{Name: "incusbr0", Type: "bridge", Managed: true, Project: "default"},
			{Name: "eth0", Type: "physical", Project: "default"},
		},
		NetworkLeases: map[string]map[string][]api.NetworkLease{
			"incusbr0": {"default": {
				{Hostname: "incusbr0.gw", Address: "192.0.2.1", Type: "GATEWAY"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "192.0.2.10", Type: "DYNAMIC"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "2001:db8::10", Type: "DYNAMIC"},
			}},
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
			"default": {{Name: "data", Type: "custom", Project: "default"}},
		},
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
	incusCommand.ServerVersion = "7.4"

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

// ready waits for startup's fetches to land and the screen to settle. The
// resources are drawn behind tabs, so it's their panels rather than the
// screen that say they've arrived.
func (s *screen) ready(t *testing.T) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		loaded := false
		s.do(t, func() error {
			loaded = s.gui.Panels.Images.List.Len() > 0 && s.gui.Panels.Networks.List.Len() > 0
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
