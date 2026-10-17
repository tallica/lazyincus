package gui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

// : starts from the selected row's project, ready for the verb.
func TestTypedCommandAsksWithTheRowsProject(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	project := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Project
	})

	s.press(t, ':')
	s.settle(t, "enter to run")
	s.settle(t, "│--project "+project+" ")
}

func TestTypedCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	})
	s.ready(t)

	s.press(t, ':')
	s.settle(t, "fake is read-only")
}

// A row on another remote names no project of this session's, so the command
// runs with none: the session's would name a project that needn't exist there.
func TestTypedCommandSeedsNoProjectForAnotherRemotesRow(t *testing.T) {
	// As a stack whose compose file couldn't be read is: no project of its own.
	broken := &commands.ComposeStack{Dir: t.TempDir(), Remote: "elsewhere"}

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.IncusCommand.UseProject("shop")
		withStacks(t, nil, broken)(s)
	})
	s.settle(t, "Stacks")

	assert.Empty(t, onLoop(t, s, func() string { return s.gui.typedCommandProject("stacks") }))
}

// The same row on the session's own remote takes the session's project.
func TestTypedCommandSeedsTheSessionsProjectForItsOwnRemote(t *testing.T) {
	broken := &commands.ComposeStack{Dir: t.TempDir()}

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.IncusCommand.UseProject("shop")
		withStacks(t, nil, broken)(s)
	})
	s.settle(t, "Stacks")

	assert.Equal(t, "shop", onLoop(t, s, func() string { return s.gui.typedCommandProject("stacks") }))
}

// A line that doesn't split is refused where it stands, saying why: running
// it would mean guessing where the quote ends. It used to take the app down.
func TestTypedCommandRefusesAnUnclosedQuote(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.press(t, ':')
	s.settle(t, "enter to run")

	for _, key := range `incus exec web -- sh -c "echo hi` {
		s.press(t, key)
	}

	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "unterminated")
}

// : is one key on any view, so the panel it acts on is the one in focus: a
// stack pinned elsewhere keeps its own remote, and the project that remote
// has for it.
func TestTypedCommandKeepsTheStackOnAnotherRemote(t *testing.T) {
	pinned := &commands.ComposeStack{Dir: t.TempDir(), Name: "shop", Remote: "site-a"}
	s := startScreenWith(t, 140, 40, nil, withStacks(t, nil, pinned))
	s.settle(t, "Stacks")
	landed(t, s, "stacks")
	waitForStack(t, s)

	s.press(t, ':')
	s.settle(t, "incus on site-a")
	s.settle(t, "--project shop ")
}

// The same stack on a read-only remote refuses : however writable the
// session's own remote is: the check follows the panel's remote, as its other
// keys do.
func TestTypedCommandRefusedOnAReadOnlyStacksRemote(t *testing.T) {
	pinned := &commands.ComposeStack{Dir: t.TempDir(), Name: "shop", Remote: "site-a"}
	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"site-a": {ReadOnly: true}}
	}, withStacks(t, nil, pinned))
	s.settle(t, "Stacks")
	landed(t, s, "stacks")
	waitForStack(t, s)

	s.press(t, ':')
	s.settle(t, "site-a is read-only")
}

// A global key reaches a popup that isn't editable too, where the prompt
// would otherwise open over the menu.
func TestTypedCommandDoesNothingInAPopup(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.press(t, 'x')
	s.settle(t, "focus main panel")
	s.press(t, ':')

	assert.NotContains(t, s.settle(t, "focus main panel"), "enter to run")
}

// waitForStack is the Stacks panel having told the gui which stack it is on,
// which is where : reads the remote from.
func waitForStack(t *testing.T, s *screen) {
	t.Helper()

	require.Eventually(t, func() bool {
		return onLoop(t, s, func() bool { return s.gui.selectedStack.Load() != nil })
	}, 5*time.Second, 20*time.Millisecond)
}

// The command line becomes the arguments, on the remote.
func TestTypedCommandCmd(t *testing.T) {
	s := startScreen(t, 140, 40, nil)

	cmd, err := s.gui.typedCommandCmd(`incus --project shop exec web -- sh -c "echo hi | wc -c"`, "pve01", nil)
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus", "--project", "shop", "exec", "web", "--", "sh", "-c", "echo hi | wc -c"}, cmd.Args)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	// The seed keeps its place, the binary typed behind it goes.
	cmd, err = s.gui.typedCommandCmd("--project shop incus list", "pve01", projectCLIArgs("shop"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus", "--project", "shop", "list"}, cmd.Args)

	cmd, err = s.gui.typedCommandCmd("incus", "pve01", nil)
	assert.NoError(t, err)
	assert.Nil(t, cmd)

	cmd, err = s.gui.typedCommandCmd("   ", "pve01", nil)
	assert.NoError(t, err)
	assert.Nil(t, cmd)
}

// The binary typed out of habit goes, with the seed in front of it or the line
// cleared back to it; an incus that is an argument stays.
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

// enter asks the line first: the seed runs nothing, and a line that can't be
// split is refused rather than run as something else.
func TestTypedLineCheck(t *testing.T) {
	check := typedLineCheck("--project shop ")

	assert.ErrorIs(t, check("--project shop"), errKeepPrompt)
	assert.ErrorIs(t, typedLineCheck(" web")("web"), errKeepPrompt)
	assert.NoError(t, check("--project shop list"))
	assert.ErrorContains(t, check(`--project shop exec web -- sh -c "echo hi`), "unterminated")

	// With no seed, an empty line is the seed: nothing typed, nothing to run.
	assert.ErrorIs(t, typedLineCheck("")(""), errKeepPrompt)
}

// ; on a service offers its name after the cursor, for the verb before it.
func TestTypedComposeCommandOffersTheService(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, stack)(s)
		labelled(s, map[string]string{"web": "web"})
	})
	s.settle(t, "Services (default)")

	s.press(t, '2')
	s.press(t, ';')
	s.settle(t, "incus-compose: default")
	s.settle(t, "│ web")

	for _, key := range "logs" {
		s.press(t, key)
	}
	s.settle(t, "│logs web")
}

func TestTypedComposeCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = "fake"

	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}, withStacks(t, nil, stack))
	s.settle(t, "Services (default)")

	s.press(t, ';')
	s.settle(t, "fake is read-only")
}

// Run in the stack's directory, on its remote.
func TestTypedComposeCmd(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	target := composeTarget{remote: "pve01", dir: "/srv/shop", project: "shop"}

	cmd, err := s.gui.typedComposeCmd(`incus-compose logs --tail 5 web`, target)
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus-compose", "logs", "--tail", "5", "web"}, cmd.Args)
	assert.Equal(t, "/srv/shop", cmd.Dir)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	cmd, err = s.gui.typedComposeCmd("incus-compose", target)
	assert.NoError(t, err)
	assert.Nil(t, cmd)
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

// ↑ steps back through the lines run before, newest first, and ↓ past the
// newest gives back what was being typed.
func TestTypedCommandRecallsItsHistory(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		require.NoError(t, s.gui.Config.RecordCommand(config.IncusHistory, "info web"))
		require.NoError(t, s.gui.Config.RecordCommand(config.IncusHistory, "list"))
		require.NoError(t, s.gui.Config.RecordCommand(config.ComposeHistory, "ps"))
	})
	s.ready(t)

	s.press(t, ':')
	s.settle(t, "enter to run")
	s.press(t, 'x')

	s.pressKey(t, tcell.KeyUp)
	s.settle(t, "│list ")
	s.pressKey(t, tcell.KeyUp)
	s.settle(t, "│info web ")
	s.pressKey(t, tcell.KeyUp)
	s.settle(t, "│info web ")

	s.pressKey(t, tcell.KeyDown)
	s.settle(t, "│list ")
	s.pressKey(t, tcell.KeyDown)
	project := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Project
	})
	assert.NotContains(t, s.settle(t, "│--project "+project+" x"), "│list ")
}
