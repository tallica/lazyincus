package tui

import (
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
	"github.com/tallica/lazyincus/pkg/i18n"
)

type options struct {
	readOnly  bool
	compose   bool
	configure func(*config.UserConfig)
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
	incus := commands.NewIncusCommandWithClient(log, osCommand, tr, appConfig, server, "fake")

	m := NewModel(incus, osCommand, tr, appConfig)
	// Whatever this machine has installed, the panels are the test's.
	m.composeAvailable = opts.compose
	m.panels = m.newPanels()
	m.focus = nil
	m.landOffStacks()

	return m
}

// start sizes m and feeds it its first fetch, the way the program would.
func start(t *testing.T, opts options, width, height int) *Model {
	t.Helper()

	m := newTestModel(t, opts)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m.Update(m.Init()())

	return m
}

func press(m *Model, keys ...string) {
	for _, key := range keys {
		if cmd := m.handleKey(key); cmd != nil {
			if msg := cmd(); msg != nil {
				m.Update(msg)
			}
		}
	}
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
	assert.Contains(t, lines[0], "Info - Config")
	assert.Contains(t, lines[1], "cache", "running instances come first, by name")
	assert.Contains(t, lines[3], "db")
}

func TestSelectionFollowsTheInstanceAcrossRefresh(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "j")
	require.Equal(t, "web", m.selectedInstance().Name)

	m.Update(m.fetchInstances()())
	assert.Equal(t, "web", m.selectedInstance().Name)
}

func TestLateFetchIsDropped(t *testing.T) {
	m := newTestModel(t, options{})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 16})

	m.fetchInstances()
	m.Update(m.fetchInstances()())
	m.Update(instancesMsg{seq: 1})

	assert.Len(t, m.instances, 3)
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
	press(m, "]")
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

	assert.Nil(t, m.handleKey("q"))
	require.Len(t, m.popups, 1)
	assert.Contains(t, screen(m)[39], "n/esc: no, y/enter: yes")

	cmd := m.handleKey("y")
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestTooSmall(t *testing.T) {
	m := start(t, options{}, 9, 20)

	assert.Contains(t, strings.Join(screen(m), ""), "Not")
}
