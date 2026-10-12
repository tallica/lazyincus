package gui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
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

// The command line becomes the arguments, on the remote.
func TestTypedCommandCmd(t *testing.T) {
	s := startScreen(t, 140, 40, nil)

	cmd, err := s.gui.typedCommandCmd(`incus --project shop exec web -- sh -c "echo hi | wc -c"`, "pve01")
	assert.NoError(t, err)
	assert.Equal(t, []string{"incus", "--project", "shop", "exec", "web", "--", "sh", "-c", "echo hi | wc -c"}, cmd.Args)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	cmd, err = s.gui.typedCommandCmd("incus", "pve01")
	assert.NoError(t, err)
	assert.Nil(t, cmd)

	cmd, err = s.gui.typedCommandCmd("   ", "pve01")
	assert.NoError(t, err)
	assert.Nil(t, cmd)
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
