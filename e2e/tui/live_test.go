package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/rs/zerolog"
	"github.com/tallica/lazyincus/e2e/internal/harness"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/tui"
)

// live is the whole program on a terminal emulator: the event stream, pollers
// and tickers run as they do for a user, which the headless model turns off.
type live struct {
	term *vt.SafeEmulator
}

func runLive(t *testing.T, run *harness.Run) *live {
	t.Helper()

	const width, height = 140, 40

	term := vt.NewSafeEmulator(width, height)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	// The emulator is the terminal both ways: it reads the program's output,
	// and its replies to queries and our keys are the program's input.
	input := newInbox()
	go func() { _, _ = io.Copy(input, term) }()

	log := zerolog.Nop()
	go func() {
		done <- tui.Run(run.Command, run.OS, i18n.NewTranslationSet(&log, "en"), run.Config,
			tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(onlcr{term}),
			tea.WithWindowSize(width, height), tea.WithoutSignals())
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the program didn't stop")
		}
		_ = term.Close()
		input.close()
	})

	l := &live{term: term}
	// Keys before the first fetch lands go to Stacks, which then hands the focus on.
	harness.WaitFor(t, within, "the first listing", func() (bool, string) {
		screen := l.screen()
		return strings.Contains(screen, "Instances") && !strings.Contains(screen, "Loading…"), screen
	})

	return l
}

// inbox is a pipe whose writes never wait for a read. The emulator writes
// keys and replies holding its lock, which the program's output needs: a
// plain pipe deadlocks once the program stops reading to write.
type inbox struct {
	mu     sync.Mutex
	ready  *sync.Cond
	buf    bytes.Buffer
	closed bool
}

func newInbox() *inbox {
	in := &inbox{}
	in.ready = sync.NewCond(&in.mu)

	return in
}

func (in *inbox) Write(p []byte) (int, error) {
	in.mu.Lock()
	defer in.mu.Unlock()

	in.buf.Write(p)
	in.ready.Broadcast()

	return len(p), nil
}

func (in *inbox) Read(p []byte) (int, error) {
	in.mu.Lock()
	defer in.mu.Unlock()

	for in.buf.Len() == 0 && !in.closed {
		in.ready.Wait()
	}

	if in.buf.Len() == 0 {
		return 0, io.EOF
	}

	return in.buf.Read(p)
}

func (in *inbox) close() {
	in.mu.Lock()
	defer in.mu.Unlock()

	in.closed = true
	in.ready.Broadcast()
}

// onlcr does what a cooked TTY would: Bubble Tea, its input no TTY,
// writes LF expecting the terminal to make it CRLF (mapNl in tea.go).
type onlcr struct{ io.Writer }

func (o onlcr) Write(p []byte) (int, error) {
	if _, err := o.Writer.Write(bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (l *live) screen() string { return ansi.Strip(l.term.Render()) }

var liveKeys = map[string]vt.KeyPressEvent{
	"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape},
	"ctrl+p": {Code: 'p', Mod: vt.ModCtrl}, "ctrl+u": {Code: 'u', Mod: vt.ModCtrl},
}

// press sends keys by name, as the headless Press takes them; anything else is typed.
func (l *live) press(keys ...string) {
	for _, key := range keys {
		if k, ok := liveKeys[key]; ok {
			l.term.SendKey(k)
		} else {
			l.term.SendText(key)
		}
	}
}

func (l *live) waitFor(t *testing.T, within time.Duration, what string, cond func(screen string) bool) {
	t.Helper()

	harness.WaitFor(t, within, what, func() (bool, string) {
		screen := l.screen()
		return cond(screen), screen
	})
}
