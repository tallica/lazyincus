package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Headless is a Model driven a message at a time, with no terminal: for tests.
type Headless struct{ m *Model }

func NewHeadless(incus *commands.IncusCommand, osCommand *commands.OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig) *Headless {
	return &Headless{m: headless(NewModel(incus, osCommand, tr, cfg))}
}

// headless turns off what would run behind the caller's back.
func headless(m *Model) *Model {
	m.after = func(time.Duration, tea.Msg) tea.Cmd { return nil }
	m.streamEvents = false
	m.runTask = runTaskHere
	m.checkUpstreams = func(context.Context, []commands.UpstreamTarget) map[string]commands.UpstreamStatus { return nil }

	return m
}

// Start sizes the screen and runs the first fetch, as the program would.
func (h *Headless) Start(width, height int) {
	h.m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	run(h.m, h.m.Init())
}

// Press types keys by name: "j", "enter", "ctrl+u".
func (h *Headless) Press(keys ...string) { press(h.m, keys...) }

func (h *Headless) Type(text string) { typeText(h.m, text) }

// Refresh is how a caller sees a change, with no events or pollers to bring it.
func (h *Headless) Refresh() { run(h.m, h.m.refreshAll()) }

func (h *Headless) Screen() []string { return screen(h.m) }

// run runs cmd and every Cmd its messages lead to, in order, as the program
// would but on this goroutine.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}

	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, cmd := range msg {
			run(m, cmd)
		}
	default:
		_, next := m.Update(msg)
		run(m, next)
	}
}

// runTaskHere runs a main task's first pass on the Cmd's goroutine, its
// writes the messages that follow: a ticker stops after its first.
func runTaskHere(task func(ctx context.Context, send func(tea.Msg))) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var writes tea.BatchMsg
		task(ctx, func(msg tea.Msg) {
			writes = append(writes, func() tea.Msg { return msg })
		})

		return writes
	}
}

// press types keys by name, as Update would take them from KeyPressMsg.
func press(m *Model, keys ...string) {
	for _, key := range keys {
		_, cmd := m.Update(keyPress(key))
		run(m, cmd)
	}
}

var namedKeys = map[string]rune{
	"esc": tea.KeyEscape, "enter": tea.KeyEnter, "tab": tea.KeyTab, "space": tea.KeySpace,
	"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

// keyPress is the KeyPressMsg whose String is name: "x", "esc", "ctrl+u".
func keyPress(name string) tea.KeyPressMsg {
	msg := tea.KeyPressMsg{}
	rest := name
	for _, mod := range []struct {
		prefix string
		mod    tea.KeyMod
	}{{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift}} {
		if after, ok := strings.CutPrefix(rest, mod.prefix); ok && after != "" {
			msg.Mod |= mod.mod
			rest = after
		}
	}

	if code, ok := namedKeys[rest]; ok {
		msg.Code = code
		// A terminal types space as it does any rune.
		if code == tea.KeySpace && msg.Mod == 0 {
			msg.Text = " "
		}
	} else {
		msg.Code = []rune(rest)[0]
		if msg.Mod == 0 {
			msg.Text = rest
		}
	}

	if msg.String() != name {
		panic("no KeyPressMsg reads " + name)
	}

	return msg
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

func typeText(m *Model, text string) {
	for _, r := range text {
		if r == ' ' {
			press(m, "space")
			continue
		}
		press(m, string(r))
	}
}
