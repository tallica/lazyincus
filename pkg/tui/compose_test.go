package tui

import (
	"os/exec"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
)

// finishing stands in for the terminal: it records cmd and has it succeed at once.
func finishing(ran *[]*exec.Cmd) func(*exec.Cmd, func(error), func(error) tea.Cmd) tea.Cmd {
	return func(cmd *exec.Cmd, after func(error), then func(error) tea.Cmd) tea.Cmd {
		*ran = append(*ran, cmd)
		if after != nil {
			after(nil)
		}

		return then(nil)
	}
}

// composeModel lists one stack, "default", with a runOnTerminal that records
// the commands it is asked to run and finishes them at once.
func composeModel(t *testing.T, opts options) (*Model, *[]*exec.Cmd, *int) {
	t.Helper()

	stack := testStack(t, t.TempDir(), "default", "web")
	opts.compose, opts.localStack = true, stack
	m := start(t, opts, 140, 40)

	var ran []*exec.Cmd

	loads := 0
	load := m.loadStack
	m.loadStack = func(dir string) *commands.ComposeStack {
		loads++
		return load(dir)
	}
	m.runOnTerminal = finishing(&ran)

	press(m, "1")

	return m, &ran, &loads
}

func TestComposeRecreateArgs(t *testing.T) {
	assert.Equal(t, []string{"up", "--recreate", "--pull", "never", "--detach"}, composeRecreateArgs("never"))
	assert.Equal(t, []string{"up", "--recreate", "--pull", "always", "--detach"}, composeRecreateArgs("always"))
}

func TestComposeCmdNarrowsToTheServiceOnItsRemote(t *testing.T) {
	m := newTestModel(t, options{})

	cmd := m.composeCmd(composeTarget{dir: "/srv/shop", remote: "far"}, "restart")
	assert.Equal(t, "/srv/shop", cmd.Dir)
	assert.Equal(t, "restart", cmd.Args[len(cmd.Args)-1])
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=far")

	cmd = m.composeCmd(composeTarget{dir: "/srv/shop", service: "web"}, "restart")
	assert.Equal(t, []string{"restart", "web"}, cmd.Args[len(cmd.Args)-2:])
}

func TestAStackVerbRunsAndReadsTheStackAgain(t *testing.T) {
	m, ran, loads := composeModel(t, options{})
	before := *loads

	press(m, "r")
	require.Len(t, *ran, 1)
	assert.Equal(t, "restart", (*ran)[0].Args[len((*ran)[0].Args)-1])
	assert.Greater(t, *loads, before, "the stack's config is read again")
	assert.Empty(t, m.popups)
}

func TestStoppingAStackAsksNamingTheProject(t *testing.T) {
	m, ran, _ := composeModel(t, options{})

	press(m, "s")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to stop project default?", ansi.Strip(m.popups[0].message))
	assert.Empty(t, *ran)

	press(m, "y")
	require.Len(t, *ran, 1)
	assert.True(t, slices.Contains((*ran)[0].Args, "stop"))
}

func TestRecreateOffersToPullFirst(t *testing.T) {
	m, _, _ := composeModel(t, options{})

	press(m, "U")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, m.tr.ComposeRecreateMenuTitle, m.topMenu().title)

	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "Are you sure you want to recreate")
	press(m, "n")

	press(m, "U", "j", "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "pull the latest images and recreate")
}

func TestRecreateStopsAskingOnA(t *testing.T) {
	m, ran, _ := composeModel(t, options{})
	assert.False(t, m.cfg.SkipsConfirm("recreate"))

	press(m, "U", "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.options(), m.tr.YesAlways)

	press(m, "a")
	assert.Empty(t, m.popups)
	assert.True(t, m.cfg.SkipsConfirm("recreate"))
	assert.False(t, m.cfg.SkipsConfirm("recreate-pull"))
	require.Len(t, *ran, 1)
	assert.Equal(t, "--detach", (*ran)[0].Args[len((*ran)[0].Args)-1])

	press(m, "U", "enter")
	assert.Empty(t, m.popups, "asked no more")
	assert.Len(t, *ran, 2)
}

func TestPausingAStackWithNothingRunningIsRefused(t *testing.T) {
	m, ran, _ := composeModel(t, options{})

	press(m, "p")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, m.tr.StackNotRunning)
	assert.Empty(t, *ran)
}

func TestComposePauseVerb(t *testing.T) {
	assert.Equal(t, "pause", composePauseVerb())
	assert.Equal(t, "pause", composePauseVerb("Running", "Frozen"))
	assert.Equal(t, "unpause", composePauseVerb("Frozen", commands.ServiceNone, "Frozen"))
}

func TestComposeVerbsAreRefusedReadOnly(t *testing.T) {
	m, ran, _ := composeModel(t, options{readOnly: true})

	for _, key := range []string{"u", "d", "U", "S", "s", "r", "p", "f", "b", "g"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}

	assert.Empty(t, *ran)
}

func TestFollowingLogsIsNotAMutation(t *testing.T) {
	m, ran, _ := composeModel(t, options{readOnly: true})

	press(m, "M")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"logs", "--follow"}, (*ran)[0].Args[len((*ran)[0].Args)-2:])
}

func TestAConfirmationNamesAStackElsewhereByItsRemote(t *testing.T) {
	m := newTestModel(t, options{})

	assert.Equal(t, "project shop", m.composeTargetName(composeTarget{project: "shop", remote: "fake"}))
	assert.Equal(t, "project shop on far", m.composeTargetName(composeTarget{project: "shop", remote: "far"}))
	assert.Equal(t, "service web on far", m.composeTargetName(composeTarget{service: "web", remote: "far"}))
}
