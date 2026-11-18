package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

// Each of these reads what the fake makes up.

// Reading the console log empties it (docs/Incus.md, "Logs"): a second read must not blank the tab.
func TestTheLogsTabKeepsWhatItHasReadOnceRead(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})

	h := open(t, run, name)
	waitForRow(t, h, name, "running")
	h.Press("]")
	waitForTab(t, h, "Welcome to Alpine Linux", "[", "]")

	h.Press("[", "]", "[", "]")

	if !strings.Contains(mainText(h), "Welcome to Alpine Linux") {
		t.Fatalf("the log went on a second read:\n%s", mainText(h))
	}
}

func TestTheInfoTabCountsARunningContainer(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})

	h := open(t, run, name)
	waitForRow(t, h, name, "running")

	counters := regexp.MustCompile(`CPU: +[0-9.]+(ms|s) total[\s\S]*Memory: +[0-9.]+[KMG]iB[\s\S]*Processes: +[1-9]`)
	waitForScreen(t, h, "CPU, memory and processes", func() bool { return counters.MatchString(mainText(h)) })
}

// Top execs ps, busybox's here, whose flags differ from util-linux's.
func TestTheTopTabListsBusyboxProcesses(t *testing.T) {
	run := harness.Get(t)
	name := run.Container(t, harness.ContainerOptions{Running: true})

	h := open(t, run, name)
	waitForRow(t, h, name, "running")
	h.Press("]", "]", "]", "]")
	waitForTab(t, h, "/sbin/getty", "[", "]")
}

// The network is the default project's; the lease, the run's (docs/Incus.md, "Network leases").
func TestTheLeasesTabListsAnotherProjectsContainer(t *testing.T) {
	run := harness.Get(t)
	if run.Network == "" {
		t.Skip("the default profile has no network")
	}

	name := run.Container(t, harness.ContainerOptions{Running: true})

	h := start(t, run)
	goTo(h, run.Network)
	waitForTab(t, h, name, "]", "[")
}

// dir sizes no volume (docs/Incus.md, "Volume usage"): the cell stays blank, not 0B.
func TestAVolumeOnDirShowsNoSize(t *testing.T) {
	run := harness.Get(t)
	if driver := run.PoolDriver(t); driver != "dir" {
		t.Skipf("the pool is %s", driver)
	}

	name := run.Volume(t)

	h := start(t, run)
	goTo(h, name)
	waitListed(t, h, name)

	size := regexp.MustCompile(`^[0-9.]+(B|KiB|MiB|GiB)$`)
	for _, field := range row(h, name) {
		if size.MatchString(field) {
			t.Fatalf("%s shows a size: %v", name, row(h, name))
		}
	}
}
