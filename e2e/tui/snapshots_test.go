package tui

import (
	"slices"
	"testing"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

func TestDeletingASnapshotRemovesIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})
	snap := run.Snapshot(t, name)

	h := open(t, run, name)
	waitListed(t, h, snap)
	goTo(h, snap)
	h.Press("d")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitGone(t, h, snap)
}

// The fake accepts a restore and changes nothing; this one has to undo a change.
func TestRestoringASnapshotUndoesWhatChangedSince(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})
	run.SetConfig(t, name, "user.e2e", "before")
	snap := run.Snapshot(t, name)
	run.SetConfig(t, name, "user.e2e", "after")

	h := open(t, run, name)
	waitListed(t, h, snap)
	goTo(h, snap)
	h.Press("r")
	requirePopup(t, h, "Confirm")
	h.Press("y")

	harness.WaitFor(t, within, "the restored config", func() (bool, string) {
		value := run.ConfigKey(t, name, "user.e2e")
		return value == "before", value
	})
}

// incus copy, which headless runs for real: it needs no terminal.
func TestANewInstanceFromASnapshotIsListed(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{})
	snap := run.Snapshot(t, name)
	copied := name + "-" + snap

	h := open(t, run, name)
	waitListed(t, h, snap)
	goTo(h, snap)
	action(h, "new instance from snapshot", "snapshots")
	requirePopup(t, h, "New instance from")
	h.Press("enter")
	waitForInfo(t, h, copied, "stopped")

	if !slices.Contains(run.InstanceNames(t), copied) {
		t.Fatalf("no %s on the daemon", copied)
	}
}
