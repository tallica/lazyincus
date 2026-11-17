package daemon

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/e2e/internal/harness"
	"github.com/tallica/lazyincus/pkg/commands"
)

// docs/Incus.md, "Delete": a frozen instance counts as running too.
func TestDeleteRefusesARunningInstance(t *testing.T) {
	r := harness.Get(t)

	for _, frozen := range []bool{false, true} {
		name := r.Container(t, harness.ContainerOptions{Running: true})
		if frozen {
			r.SetState(t, name, "freeze")
		}

		if err := r.Instance(t, name).Delete(); !errors.Is(err, commands.ErrInstanceRunning) {
			t.Errorf("frozen %v: Delete returned %v, want ErrInstanceRunning", frozen, err)
		}
	}
}

func TestForceDeleteRemovesARunningInstance(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{Running: true})

	if err := r.Instance(t, name).ForceDelete(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := r.Client.GetInstance(name); !api.StatusErrorCheck(err, http.StatusNotFound) {
		t.Errorf("after ForceDelete, GetInstance returned %v, want a 404", err)
	}
}

// docs/Incus.md, "Logs".
func TestConsoleLogDrainsOnRead(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{Running: true})

	read := func() string {
		reader, err := r.Client.GetInstanceConsoleLog(name, &incus.InstanceConsoleLogArgs{})
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}

		return string(data)
	}

	toConsole := func(marker string) {
		r.Exec(t, name, "sh", "-c", "echo "+marker+" > /dev/console")
	}

	toConsole("e2e-first")

	var drained strings.Builder
	harness.WaitFor(t, 10*time.Second, "first marker in the console log", func() (bool, string) {
		drained.WriteString(read())
		return strings.Contains(drained.String(), "e2e-first"), drained.String()
	})

	if again := read(); strings.Contains(again, "e2e-first") {
		t.Fatalf("a second read returned the marker again: %q", again)
	}

	// What lazyincus builds on it: each read added to the last.
	instance := r.Instance(t, name)
	toConsole("e2e-second")
	harness.WaitFor(t, 10*time.Second, "second marker in TailConsoleLog", func() (bool, string) {
		log, err := instance.TailConsoleLog()
		return err == nil && strings.Contains(log, "e2e-second"), log
	})

	toConsole("e2e-third")
	harness.WaitFor(t, 10*time.Second, "both markers in TailConsoleLog", func() (bool, string) {
		log, err := instance.TailConsoleLog()
		return err == nil && strings.Contains(log, "e2e-second") && strings.Contains(log, "e2e-third"), log
	})
}

// docs/Incus.md, "Operations": dropped 5s after it ends.
func TestEndedOperationIsDroppedWithinSeconds(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{Running: true})

	op, err := r.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: "stop", Force: true, Timeout: -1}, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := op.Wait(); err != nil {
		t.Fatal(err)
	}

	id := op.Get().ID
	if _, _, err := r.Client.GetOperation(id); err != nil {
		t.Fatalf("just ended, the operation isn't listed: %v", err)
	}

	harness.WaitFor(t, 15*time.Second, "the ended operation dropped", func() (bool, string) {
		_, _, err := r.Client.GetOperation(id)
		return api.StatusErrorCheck(err, http.StatusNotFound), "still there"
	})
}

// docs/Incus.md, "Rebuild": refused on any driver.
func TestRebuildRefusesAnInstanceWithSnapshots(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{})

	snap, err := r.Client.CreateInstanceSnapshot(name, api.InstanceSnapshotsPost{Name: "before"})
	if err == nil {
		err = snap.Wait()
	}

	if err != nil {
		t.Fatal(err)
	}

	instance, _, err := r.Client.GetInstance(name)
	if err != nil {
		t.Fatal(err)
	}

	op, err := r.Client.RebuildInstanceFromImage(r.Client, api.Image{Fingerprint: instance.Config["volatile.base_image"]}, name, api.InstanceRebuildPost{})
	if err == nil {
		err = op.Wait()
	}

	if err == nil {
		t.Fatal("rebuilt an instance with a snapshot")
	}
}
