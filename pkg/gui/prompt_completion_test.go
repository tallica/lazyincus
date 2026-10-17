package gui

import (
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
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
