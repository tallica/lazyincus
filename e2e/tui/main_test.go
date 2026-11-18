// Package tui drives lazyincus's model, headless, against a real daemon:
// what the screen tests can't, because only a daemon answers it.
package tui

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tallica/lazyincus/e2e/internal/harness"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/tui"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

// within is generous: a clean stop waits on the container's init.
const within = time.Minute

func start(t *testing.T, run *harness.Run) *tui.Headless {
	t.Helper()

	log := zerolog.Nop()
	h := tui.NewHeadless(run.Command, run.OS, i18n.NewTranslationSet(&log, "en"), run.Config)
	h.Start(140, 40)

	return h
}

// open starts a model filtered down to name, so other instances on the daemon don't matter.
func open(t *testing.T, run *harness.Run, name string) *tui.Headless {
	t.Helper()

	h := start(t, run)
	h.Press("/")
	h.Type(name)
	h.Press("enter")

	return h
}

// sideWidth is the side column's, read off the top border: "…╮╭…".
func sideWidth(lines []string) int {
	return len([]rune(strings.SplitAfter(lines[0], "╮")[0]))
}

// row is the fields of name's line in the side column, or nil when it isn't listed.
func row(h *tui.Headless, name string) []string {
	lines := h.Screen()
	width := sideWidth(lines)

	for _, line := range lines {
		if !strings.HasPrefix(line, "│") {
			continue
		}

		side := string([]rune(line)[:width])
		if fields := strings.Fields(strings.Trim(side, "│")); slices.Contains(fields, name) {
			return fields
		}
	}

	return nil
}

// mainText is the main panel's column, without the side panels beside it.
func mainText(h *tui.Headless) string {
	lines := h.Screen()
	width := sideWidth(lines)

	main := make([]string, 0, len(lines))
	for _, line := range lines {
		main = append(main, string([]rune(line)[width:]))
	}

	return strings.Join(main, "\n")
}

// waitForTab waits for the main panel to show text, leaving the tab and
// coming back to read it again: headless runs a tab's first pass only.
func waitForTab(t *testing.T, h *tui.Headless, text string, away, back string) {
	t.Helper()
	waitForScreen(t, h, text+" on main", func() bool {
		h.Press(away, back)
		return strings.Contains(mainText(h), text)
	})
}

func waitForRow(t *testing.T, h *tui.Headless, name, want string) {
	t.Helper()

	waitForScreen(t, h, name+" "+want, func() bool { return slices.Contains(row(h, name), want) })
}

func screenHas(h *tui.Headless, text string) bool {
	return strings.Contains(strings.Join(h.Screen(), "\n"), text)
}

// goTo focuses item on its own panel, through the palette's item rows.
func goTo(h *tui.Headless, item string) {
	h.Press("ctrl+p")
	h.Type(item)
	h.Press("enter")
}

// action runs a palette action on what panel has selected. Its row filters
// by its description and panel; space would toggle the row, so neither has one.
func action(h *tui.Headless, description, panel string) {
	h.Press("ctrl+p")
	h.Type(strings.ReplaceAll(description+panel, " ", ""))
	h.Press("enter")
}

func waitForScreen(t *testing.T, h *tui.Headless, what string, cond func() bool) {
	t.Helper()

	harness.WaitFor(t, within, what, func() (bool, string) {
		h.Refresh()
		return cond(), strings.Join(h.Screen(), "\n")
	})
}

// waitForInfo waits for Info to show name, the selected instance, as status.
func waitForInfo(t *testing.T, h *tui.Headless, name, status string) {
	t.Helper()
	waitForScreen(t, h, name+" "+status+" on Info", func() bool {
		return screenHas(h, "│Name:         "+name+" ") && screenHas(h, "│Status:       "+status+" ")
	})
}

func waitGone(t *testing.T, h *tui.Headless, name string) {
	t.Helper()
	waitForScreen(t, h, name+" gone", func() bool { return row(h, name) == nil })
}

func waitListed(t *testing.T, h *tui.Headless, name string) {
	t.Helper()
	waitForScreen(t, h, name+" listed", func() bool { return row(h, name) != nil })
}

// requirePopup checks the popup titled title is open; what it says is the screen tests'.
func requirePopup(t *testing.T, h *tui.Headless, title string) {
	t.Helper()
	requireScreen(t, h, "╭─"+title)
}

func requireScreen(t *testing.T, h *tui.Headless, text string) {
	t.Helper()

	if !screenHas(h, text) {
		t.Fatalf("no %q on\n%s", text, strings.Join(h.Screen(), "\n"))
	}
}
