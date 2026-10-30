package tui

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	m := start(t, options{}, 140, 40)
	m.completeIncus = func(on string, args []string, word string) (commands.Completion, error) {
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
	project := m.selectedInstance().Project

	press(m, ":")
	typeText(m, "incus s")
	press(m, "tab")

	assert.Equal(t, "start stop storage", m.topPrompt().subtitle)
	assert.Equal(t, "--project "+project+" incus st", m.topPrompt().input.Value())

	mutex.Lock()
	assert.Equal(t, []string{"--project", project}, asked, "the binary typed out of habit isn't the CLI's to complete")
	assert.Equal(t, "fake", remote)
	mutex.Unlock()

	press(m, "o", "r", "tab")
	assert.Equal(t, "--project "+project+" incus storage ", m.topPrompt().input.Value())
	assert.Equal(t, m.tr.TypedCommandHint, m.topPrompt().subtitle)

	press(m, "x", "tab")
	assert.Equal(t, m.tr.NoCompletions, m.topPrompt().subtitle)
}

// tab on ; offers the verbs, then the stack's services, and completes the
// word at the cursor with the service after it left alone.
func TestTypedComposeCommandCompletesVerbsAndServices(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web", "worker")
	m := start(t, options{compose: true, localStack: stack}, 140, 40)
	press(m, "1", "2", ";")
	require.NotNil(t, m.topPrompt())
	input := func() string { return m.topPrompt().input.Value() }

	typeText(m, "unp")
	press(m, "tab")
	assert.Equal(t, "unpause web", input())

	press(m, "end", "space", "w", "tab")
	assert.Equal(t, "web worker", m.topPrompt().subtitle)
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
	m := pinnedStack(t, nil)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m.completeIncus = func(string, []string, string) (commands.Completion, error) {
		return commands.Completion{Candidates: []string{
			"instance-one", "instance-two", "instance-three", "instance-four", "instance-five",
		}}, nil
	}

	press(m, ":")
	border := promptBorder(t, m)
	assert.Contains(t, border, "─incus on site-a─", "the hint, cut, kept off the title")
	assert.Contains(t, border, "…")

	typeText(m, "start i")
	press(m, "tab")
	border = promptBorder(t, m)
	assert.Contains(t, border, "─incus on site-a─")
	assert.Contains(t, border, "instance-one")
	assert.Contains(t, border, "…")
}

// promptBorder is the screen's line holding the prompt's top border.
func promptBorder(t *testing.T, m *Model) string {
	t.Helper()

	for _, line := range screen(m) {
		if strings.Contains(line, "incus on") {
			return line
		}
	}

	t.Fatalf("no prompt on screen:\n%s", popupText(m))

	return ""
}

func TestFitBeside(t *testing.T) {
	assert.Equal(t, "enter to run", fitBeside("enter to run", "incus", 79))
	assert.Equal(t, "enter to…", fitBeside("enter to run", "incus", 24))
	assert.Empty(t, fitBeside("enter to run", "incus on a-long-remote", 29))
}

func popupWidth(m *Model) int {
	x0, x1 := m.popupX(m.topPrompt())

	return x1 - x0
}

// A long list is cut to the popup as it stands rather than widening it, which
// would have the prompt jump on every tab.
func TestCompletionKeepsThePromptsWidth(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.completeIncus = func(string, []string, string) (commands.Completion, error) {
		var completion commands.Completion
		for i := range 20 {
			completion.Candidates = append(completion.Candidates, fmt.Sprintf("instance-%02d", i))
		}

		return completion, nil
	}

	press(m, ":")
	m.fitViews()
	before := popupWidth(m)

	press(m, "tab")
	m.fitViews()

	assert.Contains(t, m.topPrompt().subtitle, "instance-00")
	assert.Contains(t, m.topPrompt().fittedSubtitle, "…")
	assert.Equal(t, before, popupWidth(m))
}

// Nor does it narrow: the hint widens the popup past half the screen, and the
// shorter candidates in its place leave it where it was.
func TestCompletionDoesntNarrowThePrompt(t *testing.T) {
	m := start(t, options{}, 100, 40)
	m.completeIncus = func(string, []string, string) (commands.Completion, error) {
		return commands.Completion{Candidates: []string{"start", "stop"}}, nil
	}

	press(m, ":")
	m.fitViews()
	before := popupWidth(m)
	require.Greater(t, before, 50, "the hint should widen the popup for this to test anything")

	press(m, "s", "tab")
	m.fitViews()

	assert.Equal(t, "start stop", m.topPrompt().subtitle)
	assert.Equal(t, before, popupWidth(m))
}

// The candidates are for the line tab was pressed on: typing on puts the hint
// back rather than leaving them up for a line they no longer answer.
func TestEditingAfterTabPutsTheHintBack(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.completeIncus = func(string, []string, string) (commands.Completion, error) {
		return commands.Completion{Candidates: []string{"start", "stop"}}, nil
	}

	press(m, ":", "s", "tab")
	require.Equal(t, "start stop", m.topPrompt().subtitle)

	press(m, "t")

	assert.Equal(t, m.tr.TypedCommandHint, m.topPrompt().subtitle)
}

// An answer for a line that has been typed on, or a prompt closed, is dropped.
func TestACompletionForAnEditedLineIsDropped(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.completeIncus = func(string, []string, string) (commands.Completion, error) {
		return commands.Completion{Candidates: []string{"start", "stop"}}, nil
	}

	press(m, ":", "s")
	_, cmd := m.Update(keyPress("tab"))
	press(m, "t")

	run(m, cmd)

	assert.Equal(t, m.tr.TypedCommandHint, m.topPrompt().subtitle)
	assert.True(t, strings.HasSuffix(m.topPrompt().input.Value(), "st"))
}
