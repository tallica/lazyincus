package tui

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestTypedCommandAsksWithTheRowsProject(t *testing.T) {
	m := start(t, options{}, 140, 40)
	project := m.selectedInstance().Project

	press(m, ":")

	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "incus", m.topPrompt().title)
	assert.Equal(t, m.tr.TypedCommandHint, m.topPrompt().subtitle)
	assert.Equal(t, "--project "+project+" ", m.topPrompt().input.Value())
}

func TestTypedCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	m := start(t, options{configure: func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}}, 140, 40)

	press(m, ":")

	assert.Nil(t, m.topPrompt())
	assert.Contains(t, popupText(m), "fake is read-only")
}

// A row on another remote names no project of this session's.
func TestTypedCommandSeedsNoProjectForAnotherRemotesRow(t *testing.T) {
	broken := &commands.ComposeStack{Dir: t.TempDir(), Remote: "elsewhere"}
	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{broken}}, 140, 40)
	m.incus.UseProject("shop")
	press(m, "1")
	require.Equal(t, "stacks", m.currentView())
	m.selectedStack = broken

	assert.Empty(t, m.typedCommandProject("stacks"))
}

func TestTypedCommandSeedsTheSessionsProjectForItsOwnRemote(t *testing.T) {
	broken := &commands.ComposeStack{Dir: t.TempDir()}
	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{broken}}, 140, 40)
	m.incus.UseProject("shop")
	press(m, "1")

	assert.Equal(t, "shop", m.typedCommandProject("stacks"))
}

// A line that doesn't split is refused where it stands, saying why.
func TestTypedCommandRefusesAnUnclosedQuote(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, ":")
	typeText(m, `incus exec web -- sh -c "echo hi`)
	press(m, "enter")

	require.NotNil(t, m.topPrompt())
	assert.Contains(t, m.topPrompt().subtitle, "unterminated")
}

// A stack pinned elsewhere keeps its own remote, and the project that remote has for it.
func TestTypedCommandKeepsTheStackOnAnotherRemote(t *testing.T) {
	m := pinnedStack(t, nil)

	press(m, ":")

	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "incus on site-a", m.topPrompt().title)
	assert.Equal(t, "--project shop ", m.topPrompt().input.Value())
}

func TestTypedCommandRefusedOnAReadOnlyStacksRemote(t *testing.T) {
	m := pinnedStack(t, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"site-a": {ReadOnly: true}}
	})

	press(m, ":")

	assert.Nil(t, m.topPrompt())
	assert.Contains(t, popupText(m), "site-a is read-only")
}

func pinnedStack(t *testing.T, configure func(*config.UserConfig)) *Model {
	t.Helper()

	pinned := &commands.ComposeStack{Dir: t.TempDir(), Name: "shop", Remote: "site-a"}
	m := start(t, options{
		compose: true, savedStacks: []*commands.ComposeStack{pinned}, configure: configure,
		remotes: map[string]*incustest.Server{"site-a": incustest.New(incustest.Server{})},
	}, 140, 40)
	m.loadStack = func(dir string) *commands.ComposeStack { return pinned }
	press(m, "1")
	untilRemotes(t, m, func() bool { return m.selectedStack != nil })

	return m
}

func TestTypedCommandDoesNothingInAPopup(t *testing.T) {
	m := start(t, options{}, 140, 40)

	press(m, "x", ":")

	assert.Len(t, m.popups, 1)
	assert.Nil(t, m.topPrompt())
}

func TestTypedCommandRunsOnTheTerminalAndIsRemembered(t *testing.T) {
	m := start(t, options{}, 140, 40)
	var ran []*exec.Cmd
	m.runOnTerminal = finishing(&ran)
	project := m.selectedInstance().Project

	press(m, ":")
	typeText(m, "incus list")
	press(m, "enter")

	require.Len(t, ran, 1)
	assert.Equal(t, []string{"incus", "--project", project, "list"}, ran[0].Args)
	assert.Empty(t, m.popups)

	lines, err := m.cfg.LoadHistory(config.IncusHistory)
	require.NoError(t, err)
	assert.Equal(t, []string{"--project " + project + " incus list"}, lines)
}

func TestTypedCommandOnTheSeedAloneRunsNothing(t *testing.T) {
	m := start(t, options{}, 140, 40)
	var ran []*exec.Cmd
	m.runOnTerminal = finishing(&ran)

	press(m, ":", "enter")

	assert.NotNil(t, m.topPrompt())
	assert.Empty(t, ran)
}

func TestTypedCommandCmd(t *testing.T) {
	m := newTestModel(t, options{})

	cmd, err := m.typedCommandCmd(`incus --project shop exec web -- sh -c "echo hi | wc -c"`, "pve01", nil)
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus", "--project", "shop", "exec", "web", "--", "sh", "-c", "echo hi | wc -c"}, cmd.Args)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	// The seed keeps its place, the binary typed behind it goes.
	cmd, err = m.typedCommandCmd("--project shop incus list", "pve01", projectCLIArgs("shop"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus", "--project", "shop", "list"}, cmd.Args)

	cmd, err = m.typedCommandCmd("incus", "pve01", nil)
	assert.NoError(t, err)
	assert.Nil(t, cmd)

	cmd, err = m.typedCommandCmd("   ", "pve01", nil)
	assert.NoError(t, err)
	assert.Nil(t, cmd)
}

func TestTypedArgsDropsTheBinary(t *testing.T) {
	seed := projectCLIArgs("shop")

	for line, want := range map[string][]string{
		"--project shop incus list":           {"--project", "shop", "list"},
		"--project shop list":                 {"--project", "shop", "list"},
		"--project shop exec web -- incus ls": {"--project", "shop", "exec", "web", "--", "incus", "ls"},
	} {
		got, err := typedArgs(line, seed, "incus")
		assert.NoError(t, err, line)
		assert.Equal(t, want, got, line)
	}

	got, err := typedArgs("incus list", nil, "incus")
	assert.NoError(t, err)
	assert.Equal(t, []string{"list"}, got)
}

func TestTypedLineCheck(t *testing.T) {
	check := typedLineCheck("--project shop ")

	assert.ErrorIs(t, check("--project shop"), errKeepPrompt)
	assert.ErrorIs(t, typedLineCheck(" web")("web"), errKeepPrompt)
	assert.NoError(t, check("--project shop list"))
	assert.ErrorContains(t, check(`--project shop exec web -- sh -c "echo hi`), "unterminated")

	// With no seed, an empty line is the seed: nothing typed, nothing to run.
	assert.ErrorIs(t, typedLineCheck("")(""), errKeepPrompt)
}

// Quotes group and drop; a backslash stays part of the argument it's in.
func TestSplitArgs(t *testing.T) {
	for line, want := range map[string][]string{
		"":                                 nil,
		"  list   --project  shop ":        {"list", "--project", "shop"},
		`exec web -- sh -c "echo hi | wc"`: {"exec", "web", "--", "sh", "-c", "echo hi | wc"},
		`exec web -- echo "it's"`:          {"exec", "web", "--", "echo", "it's"},
		`exec web -- sh -c "a 'b' c"`:      {"exec", "web", "--", "sh", "-c", "a 'b' c"},
		`exec web -- sed -i 's/\t/,/g' x`:  {"exec", "web", "--", "sed", "-i", `s/\t/,/g`, "x"},
		`exec web -- grep -E '\d+' /log`:   {"exec", "web", "--", "grep", "-E", `\d+`, "/log"},
		`list ""`:                          {"list", ""},
	} {
		got, err := splitArgs(line)
		assert.NoError(t, err, line)
		assert.Equal(t, want, got, line)
	}
}

func TestSplitArgsRefusesAnUnclosedQuote(t *testing.T) {
	_, err := splitArgs(`exec web -- sh -c "echo hi`)
	assert.ErrorContains(t, err, "unterminated")

	_, err = splitArgs(`exec web -- echo don't`)
	assert.ErrorContains(t, err, "unterminated")
}

// ; on a service offers its name after the cursor, for the verb before it.
func TestTypedComposeCommandOffersTheService(t *testing.T) {
	m, ran, _ := composeModel(t, options{})

	press(m, "2", ";")

	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "incus-compose: default", m.topPrompt().title)
	assert.Equal(t, " web", m.topPrompt().input.Value())
	assert.Equal(t, 0, m.topPrompt().input.Position())

	typeText(m, "logs")
	assert.Equal(t, "logs web", m.topPrompt().input.Value())

	press(m, "enter")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"incus-compose", "logs", "web"}, (*ran)[0].Args)
	assert.Empty(t, m.popups)

	lines, err := m.cfg.LoadHistory(config.ComposeHistory)
	require.NoError(t, err)
	assert.Equal(t, []string{"logs web"}, lines)
}

func TestTypedComposeCommandOnAStackOffersNoService(t *testing.T) {
	m, _, _ := composeModel(t, options{})

	press(m, ";")

	require.NotNil(t, m.topPrompt())
	assert.Empty(t, m.topPrompt().input.Value())
}

func TestTypedComposeCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	m, _, _ := composeModel(t, options{configure: func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}})

	for _, view := range []string{"1", "2"} {
		press(m, view, ";")

		assert.Nil(t, m.topPrompt())
		assert.Contains(t, popupText(m), "fake is read-only", view)
		press(m, "esc")
	}
}

// Run in the stack's directory, on its remote.
func TestTypedComposeCmd(t *testing.T) {
	m := newTestModel(t, options{})
	target := composeTarget{remote: "pve01", dir: "/srv/shop", project: "shop"}

	cmd, err := m.typedComposeCmd(`incus-compose logs --tail 5 web`, target)
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus-compose", "logs", "--tail", "5", "web"}, cmd.Args)
	assert.Equal(t, "/srv/shop", cmd.Dir)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	cmd, err = m.typedComposeCmd("incus-compose", target)
	assert.NoError(t, err)
	assert.Nil(t, cmd)
}
