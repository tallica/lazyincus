package tui

import (
	"testing"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

// None of these deletes is in the fake.

func TestDeletingAnUnusedProfileRemovesIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Profile(t)

	h := start(t, run)
	goTo(h, name)
	h.Press("d")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitGone(t, h, name)
}

func TestDeletingAnUnusedNetworkRemovesIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Bridge(t)

	h := start(t, run)
	goTo(h, name)
	h.Press("d")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitGone(t, h, name)
}

func TestDeletingAnUnusedVolumeRemovesIt(t *testing.T) {
	run := harness.Get(t)
	name := run.Volume(t)

	h := start(t, run)
	goTo(h, name)
	h.Press("d")
	requirePopup(t, h, "Confirm")
	h.Press("y")
	waitGone(t, h, name)
}

func TestAVolumeSnapshotIsCreatedThenDeleted(t *testing.T) {
	run := harness.Get(t)
	volume := run.Volume(t)
	snap := "s-" + harness.Suffix()

	h := start(t, run)
	goTo(h, volume)
	h.Press("n")
	requireScreen(t, h, "New snapshot of "+volume)
	h.Press("ctrl+u")
	h.Type(snap)
	h.Press("enter")
	waitListed(t, h, snap)

	goTo(h, snap)
	h.Press("d", "y")
	waitGone(t, h, snap)
}
