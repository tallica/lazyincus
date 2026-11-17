package daemon

import (
	"slices"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/e2e/internal/harness"
	"github.com/tallica/lazyincus/pkg/commands"
)

// docs/Incus.md, "Events": each action's operation, by the description
// operationStatuses matches, ends naming the instance, and its lifecycle
// event follows.
func TestActionsAnnounceThemselves(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{})
	events := r.Listen(t)

	// A clean stop is a shutdown; only a forced one says stopped.
	for _, step := range []struct {
		action      string
		force       bool
		description string
		lifecycle   string
	}{
		{"start", false, "Starting instance", api.EventLifecycleInstanceStarted},
		{"freeze", false, "Freezing instance", api.EventLifecycleInstancePaused},
		{"unfreeze", false, "Unfreezing instance", api.EventLifecycleInstanceResumed},
		{"restart", false, "Restarting instance", api.EventLifecycleInstanceRestarted},
		{"stop", false, "Stopping instance", api.EventLifecycleInstanceShutdown},
		{"start", false, "Starting instance", api.EventLifecycleInstanceStarted},
		{"stop", true, "Stopping instance", api.EventLifecycleInstanceStopped},
	} {
		before := len(events.All())

		op, err := r.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: step.action, Force: step.force, Timeout: -1}, "")
		if err == nil {
			err = op.Wait()
		}

		if err != nil {
			t.Fatalf("%s: %v", step.action, err)
		}

		harness.WaitFor(t, 10*time.Second, step.action+" announced", func() (bool, string) {
			since := events.Since(before)
			ended := slices.ContainsFunc(since, func(e commands.Event) bool {
				return e.Type == api.EventTypeOperation && e.Action == step.description &&
					e.Status == api.Success && slices.Contains(e.Instances, name)
			})
			announced := slices.ContainsFunc(since, func(e commands.Event) bool {
				return e.Type == api.EventTypeLifecycle && e.Action == step.lifecycle
			})

			return ended && announced, events.Actions()
		})
	}
}

// docs/Incus.md, "Events": an instance snapshot's edit sends nothing.
func TestInstanceSnapshotEditSendsNoEvent(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{})

	op, err := r.Client.CreateInstanceSnapshot(name, api.InstanceSnapshotsPost{Name: "snap"})
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatal(err)
	}

	events := r.Listen(t)

	edit, err := r.Client.UpdateInstanceSnapshot(name, "snap", api.InstanceSnapshotPut{ExpiresAt: time.Now().Add(24 * time.Hour)}, "")
	if err == nil {
		err = edit.Wait()
	}

	if err != nil {
		t.Fatal(err)
	}

	// An edit that does send one, so the stream has caught up past the first.
	instance, etag, err := r.Client.GetInstance(name)
	if err != nil {
		t.Fatal(err)
	}

	put := instance.Writable()
	put.Description = "e2e"

	update, err := r.Client.UpdateInstance(name, put, etag)
	if err == nil {
		err = update.Wait()
	}

	if err != nil {
		t.Fatal(err)
	}

	harness.WaitFor(t, 10*time.Second, "instance-updated", func() (bool, string) {
		return events.Has(func(e commands.Event) bool { return e.Action == "instance-updated" }), events.Actions()
	})

	if events.Has(func(e commands.Event) bool { return e.Action == "instance-snapshot-updated" }) {
		t.Fatalf("the daemon now announces a snapshot edit: %s", events.Actions())
	}
}

// docs/Incus.md, "Events": a volume snapshot's edit does.
func TestVolumeSnapshotEditSendsEvent(t *testing.T) {
	r := harness.Get(t)
	volume := r.Volume(t)

	op, err := r.Client.CreateStoragePoolVolumeSnapshot(r.Pool, "custom", volume, api.StorageVolumeSnapshotsPost{Name: "snap"})
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatal(err)
	}

	events := r.Listen(t)

	err = r.Client.UpdateStoragePoolVolumeSnapshot(r.Pool, "custom", volume, "snap",
		api.StorageVolumeSnapshotPut{Description: "e2e"}, "")
	if err != nil {
		t.Fatal(err)
	}

	harness.WaitFor(t, 10*time.Second, "storage-volume-snapshot-updated", func() (bool, string) {
		return events.Has(func(e commands.Event) bool { return e.Action == "storage-volume-snapshot-updated" }), events.Actions()
	})
}
