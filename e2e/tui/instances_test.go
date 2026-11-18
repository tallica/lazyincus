package tui

import (
	"testing"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

// A smoke test of the wiring: the real remote and project, not the fake's.
func TestAContainerIsListedWithItsProject(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})

	h := open(t, run, name)
	waitForRow(t, h, name, "stopped")

	for _, want := range []string{"Remote:       " + run.Remote, "Project:      " + run.Project, "Name:         " + name} {
		if !screenHas(h, want) {
			t.Errorf("Info lacks %q", want)
		}
	}
}

// The fake ignores how the state change is asked for: Timeout, Force.
func TestAContainerStartsThenStopsOnceConfirmed(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})

	h := open(t, run, name)
	waitForRow(t, h, name, "stopped")
	h.Press("S")
	waitForRow(t, h, name, "running")
	h.Press("s")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitForRow(t, h, name, "stopped")
}

// The fake has no DeleteInstance: only a daemon answers these two.
func TestDeletingAStoppedContainerRemovesIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})

	h := open(t, run, name)
	waitListed(t, h, name)
	h.Press("d")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitGone(t, h, name)
}

func TestDeletingARunningContainerOffersToForce(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})

	h := open(t, run, name)
	waitListed(t, h, name)
	h.Press("d", "y")
	requireScreen(t, h, "still running, so Incus refused")
	h.Press("y")
	waitGone(t, h, name)
}

// The fake renames a running instance in place; Incus refuses to.
func TestRenamingARunningContainerStopsAndStartsIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})
	renamed := name + "-r"

	h := open(t, run, name)
	waitListed(t, h, name)
	action(h, "rename", "instances")
	h.Press("ctrl+u")
	h.Type(renamed)
	h.Press("enter")
	requireScreen(t, h, "Incus renames only a stopped instance.")
	h.Press("y")

	// Followed to its new name, which the list may cut short.
	waitForInfo(t, h, renamed, "running")
}
