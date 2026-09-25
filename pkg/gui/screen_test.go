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
				InstancePut: api.InstancePut{Architecture: "x86_64", Profiles: []string{"default"}},
				ExpandedConfig: map[string]string{
					"image.description": "Alpine 3.22 amd64",
				},
			},
			State: &api.InstanceState{
				Status:    "Running",
				Processes: 12,
				Network: map[string]api.InstanceStateNetwork{
					"eth0": {Addresses: []api.InstanceStateNetworkAddress{
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

	return &incustest.Server{
		Instances: []api.InstanceFull{
			running("default", "web", "192.0.2.10", 2),
			stopped,
			running("default", "a-name-long-enough-to-be-cut-off", "192.0.2.144", 0),
		},
		Images: []api.Image{{
			Fingerprint: "0123456789abcdef0123456789abcdef", Project: "default", Type: "container",
			Properties: map[string]string{"description": "Alpine 3.22 amd64"},
		}},
		Networks: []api.Network{{Name: "incusbr0", Type: "bridge", Managed: true, Project: "default"}},
		Volumes: map[string][]api.StorageVolume{
			"default": {{Name: "data", Type: "custom", Project: "default"}},
		},
	}
}

type screen struct {
	gui  *Gui
	g    *gocui.Gui
	done chan error
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
	incusCommand := commands.NewIncusCommandWithClient(log, osCommand, tr, appConfig, fixtureServer(), "fake")
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

	s := &screen{gui: gui, g: g, done: make(chan error, 1)}

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
	assertGolden(t, "normal-140x40", s.settle(t, "incusbr0"))
}

func TestScreenHalf(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, s.gui.nextScreenMode)
	assertGolden(t, "half-140x40", s.settle(t, "192.0.2.10"))
}

func TestScreenFull(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, s.gui.nextScreenMode)
	s.do(t, s.gui.nextScreenMode)
	assertGolden(t, "full-140x40", s.settle(t, "192.0.2.10"))
}

func TestScreenPortrait(t *testing.T) {
	s := startScreen(t, 84, 50, nil)
	assertGolden(t, "portrait-84x50", s.settle(t, "incusbr0"))
}

func TestScreenExpandedSidePanel(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, s.gui.toggleExpandSidePanel)
	assertGolden(t, "expanded-140x40", s.settle(t, "incusbr0"))
}

func TestScreenMenu(t *testing.T) {
	s := startScreen(t, 90, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, func() error { return s.gui.handleCreateOptionsMenu(s.g, s.gui.Views.Instances) })
	assertGolden(t, "menu-90x40", s.settle(t, "focus networks panel"))
}

func TestScreenConfirmation(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, func() error {
		return s.gui.createConfirmationPanel("Confirm", "Are you sure you want to stop web?", nil, nil)
	})
	assertGolden(t, "confirmation-140x40", s.settle(t, "stop web?"))
}

func TestScreenErrorPopup(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.settle(t, "incusbr0")
	s.do(t, func() error { return s.gui.createErrorPanel("instance is running") })
	assertGolden(t, "error-140x40", s.settle(t, "instance is running"))
}

func TestScreenBorders(t *testing.T) {
	for _, border := range []string{"rounded", "single", "double", "hidden"} {
		t.Run(border, func(t *testing.T) {
			s := startScreen(t, 100, 30, func(c *config.UserConfig) { c.Gui.Border = border })
			assertGolden(t, "border-"+border+"-100x30", s.settle(t, "incusbr0"))
		})
	}
}
