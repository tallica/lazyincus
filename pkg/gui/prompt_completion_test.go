package gui

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
)

// tab on : asks the incus CLI on the panel's remote with what comes before
// the word; several candidates extend it as far as they agree and are listed,
// one finishes it.
func TestTypedCommandCompletesFromTheCLI(t *testing.T) {
	var (
		mutex  sync.Mutex
		asked  []string
		remote string
	)

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.completeIncus = func(on string, args []string, word string) (commands.Completion, error) {
			mutex.Lock()
			defer mutex.Unlock()
			asked, remote = args, on

			var completion commands.Completion
			for _, verb := range []string{"start", "stop", "storage"} {
				if strings.HasPrefix(verb, word) {
					completion.Candidates = append(completion.Candidates, verb)
				}
			}

			return completion, nil
		}
	})
	s.ready(t)

	project := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Project
	})

	s.press(t, ':')
	s.settle(t, "enter to run")

	for _, key := range "incus s" {
		s.press(t, key)
	}
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "start stop storage")
	s.settle(t, "│--project "+project+" incus st ")

	mutex.Lock()
	assert.Equal(t, []string{"--project", project}, asked, "the binary typed out of habit isn't the CLI's to complete")
	assert.Equal(t, "fake", remote)
	mutex.Unlock()

	s.press(t, 'o')
	s.press(t, 'r')
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "│--project "+project+" incus storage ")

	s.press(t, 'x')
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "nothing to complete")
}

// tab on ; offers the verbs, then the stack's services, and completes the
// word at the cursor with the service after it left alone.
func TestTypedComposeCommandCompletesVerbsAndServices(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web", "worker")
	stack.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, stack)(s)
		labelled(s, map[string]string{"web": "web"})
	})
	s.settle(t, "Services (default)")

	s.press(t, '2')
	s.press(t, ';')
	s.settle(t, "│ web")

	for _, key := range "unp" {
		s.press(t, key)
	}
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "│unpause web")

	s.pressKey(t, tcell.KeyEnd)
	s.press(t, ' ')
	s.press(t, 'w')
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "web worker")
}

func TestCompletionWord(t *testing.T) {
	args, word, ok := completionWord("--project shop st")
	assert.True(t, ok)
	assert.Equal(t, []string{"--project", "shop"}, args)
	assert.Equal(t, "st", word)

	args, word, ok = completionWord("start ")
	assert.True(t, ok)
	assert.Equal(t, []string{"start"}, args)
	assert.Empty(t, word)

	_, _, ok = completionWord(`exec web -- sh -c "ec`)
	assert.False(t, ok, "an unclosed quote")

	_, _, ok = completionWord(`file push "my fi"`)
	assert.False(t, ok, "a quoted word")
}

// On a narrow screen the candidates, and the hint before them, give way to
// the title: it's what says which remote the command runs on.
func TestCompletionLeavesTheTitleWholeOnANarrowScreen(t *testing.T) {
	pinned := &commands.ComposeStack{Dir: t.TempDir(), Name: "shop", Remote: "site-a"}
	s := startScreenWith(t, 60, 30, nil, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		s.gui.completeIncus = func(string, []string, string) (commands.Completion, error) {
			return commands.Completion{Candidates: []string{
				"instance-one", "instance-two", "instance-three", "instance-four", "instance-five",
			}}, nil
		}
	})
	s.settle(t, "Stacks")
	landed(t, s, "stacks")
	waitForStack(t, s)

	s.press(t, ':')
	border := promptBorder(t, s.settle(t, "incus on site-a"))
	assert.Contains(t, border, "─incus on site-a─", "the hint, cut, kept off the title")
	assert.Contains(t, border, "…")

	for _, key := range "start i" {
		s.press(t, key)
	}
	s.pressKey(t, tcell.KeyTab)
	border = promptBorder(t, s.settle(t, "instance-"))
	assert.Contains(t, border, "─incus on site-a─")
	assert.Contains(t, border, "instance-one")
	assert.Contains(t, border, "…")
}

// promptBorder is the screen's line holding the prompt's top border.
func promptBorder(t *testing.T, screen string) string {
	t.Helper()

	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "incus on") {
			return line
		}
	}

	t.Fatalf("no prompt on screen:\n%s", screen)

	return ""
}

func TestFitBeside(t *testing.T) {
	assert.Equal(t, "enter to run", fitBeside("enter to run", "incus", 79))
	assert.Equal(t, "enter to…", fitBeside("enter to run", "incus", 24))
	assert.Empty(t, fitBeside("enter to run", "incus on a-long-remote", 29))
}

// A long list is cut to the popup as it stands rather than widening it, which
// would have the prompt jump on every tab.
func TestCompletionKeepsThePromptsWidth(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		s.gui.completeIncus = func(string, []string, string) (commands.Completion, error) {
			var completion commands.Completion
			for i := range 20 {
				completion.Candidates = append(completion.Candidates, fmt.Sprintf("instance-%02d", i))
			}

			return completion, nil
		}
	})
	s.ready(t)

	width := func() int {
		return onLoop(t, s, func() int {
			x0, _, x1, _ := s.gui.Views.Confirmation.Dimensions()
			return x1 - x0
		})
	}

	s.press(t, ':')
	s.settle(t, "enter to run")
	before := width()

	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "instance-00")
	s.settle(t, "…")
	assert.Equal(t, before, width())
}

// Nor does it narrow: the hint widens the popup past half the screen, and the
// shorter candidates in its place leave it where it was.
func TestCompletionDoesntNarrowThePrompt(t *testing.T) {
	s := startScreenWith(t, 100, 40, nil, func(s *screen) {
		s.gui.completeIncus = func(string, []string, string) (commands.Completion, error) {
			return commands.Completion{Candidates: []string{"start", "stop"}}, nil
		}
	})
	s.ready(t)

	width := func() int {
		return onLoop(t, s, func() int {
			x0, _, x1, _ := s.gui.Views.Confirmation.Dimensions()
			return x1 - x0
		})
	}

	s.press(t, ':')
	s.settle(t, "enter to run")
	before := width()
	require.Greater(t, before, 50, "the hint should widen the popup for this to test anything")

	s.press(t, 's')
	s.pressKey(t, tcell.KeyTab)
	s.settle(t, "start stop")
	assert.Equal(t, before, width())
}
