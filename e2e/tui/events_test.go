package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

// The daemon reports Running all through a stop (docs/Incus.md, "Status
// mid-action"), so only the operation's event can mark the row.

const slowStop = 8

func rowHas(name, status string) func(string) bool {
	return func(screen string) bool { return slices.Contains(rowIn(strings.Split(screen, "\n"), name), status) }
}

// filtered runs the program with the instances filtered down to name.
func filtered(t *testing.T, run *harness.Run, name string) *live {
	t.Helper()

	l := runLive(t, run)
	l.press("/", name, "enter")

	return l
}

func TestAStopStartedElsewhereMarksTheRow(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})
	run.SlowStop(t, name, slowStop)

	l := filtered(t, run, name)
	l.waitFor(t, within, name+" running", rowHas(name, "running"))

	wait := run.Stop(t, name)
	l.waitFor(t, 5*time.Second, name+" stopping", rowHas(name, "stopping"))
	l.waitFor(t, within, name+" stopped", rowHas(name, "stopped"))

	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

func TestAStopKeepsItsMarkThroughThePolls(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})
	run.SlowStop(t, name, slowStop)

	l := filtered(t, run, name)
	l.waitFor(t, within, name+" running", rowHas(name, "running"))
	l.press("s", "y")
	l.waitFor(t, 5*time.Second, name+" stopping", rowHas(name, "stopping"))

	// Past the instances' 2s poll, which still reads Running.
	time.Sleep(3 * time.Second)

	if screen := l.screen(); !rowHas(name, "stopping")(screen) {
		t.Fatalf("the mark came off mid-stop:\n%s", screen)
	}

	l.waitFor(t, within, name+" stopped", rowHas(name, "stopped"))
}

// Resources are polled once a minute while the stream is open: only the
// profile-created event brings a new one within seconds.
func TestAProfileCreatedElsewhereShowsWithinSeconds(t *testing.T) {
	run := harness.Get(t)

	l := runLive(t, run)
	l.press("ctrl+p", "defaultprofiles", "enter")
	l.waitFor(t, 5*time.Second, "a profile's tabs", func(screen string) bool {
		return strings.Contains(screen, "─Devices - Config─")
	})

	// The stream opens a moment after the first screen.
	time.Sleep(2 * time.Second)

	name := run.Profile(t)
	l.press("/", name, "enter")
	l.waitFor(t, 5*time.Second, name+" listed", func(screen string) bool {
		return rowIn(strings.Split(screen, "\n"), name) != nil
	})
}
