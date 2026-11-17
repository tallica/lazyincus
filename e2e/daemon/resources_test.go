package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/e2e/internal/harness"
)

// docs/Incus.md, "Deletes the daemon refuses".
func TestDefaultProfileCannotBeDeletedOrRenamed(t *testing.T) {
	r := harness.Get(t)

	if err := r.Client.DeleteProfile("default"); err == nil {
		t.Error("deleted the default profile")
	}

	if err := r.Client.RenameProfile("default", api.ProfilePost{Name: "e2e-renamed"}); err == nil {
		t.Error("renamed the default profile")
	}
}

func TestProfileInUseCannotBeDeleted(t *testing.T) {
	r := harness.Get(t)
	profile := "p-" + harness.Suffix()

	if err := r.Client.CreateProfile(api.ProfilesPost{Name: profile}); err != nil {
		t.Fatal(err)
	}

	r.Container(t, harness.ContainerOptions{Profiles: []string{profile}})

	err := r.Client.DeleteProfile(profile)
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("deleting a profile in use returned %v, want a refusal saying it's in use", err)
	}
}

func TestImageInUseCanBeDeleted(t *testing.T) {
	r := harness.Get(t)
	name := r.Container(t, harness.ContainerOptions{})

	instance, _, err := r.Client.GetInstance(name)
	if err != nil {
		t.Fatal(err)
	}

	op, err := r.Client.DeleteImage(instance.Config["volatile.base_image"])
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatalf("deleting the image an instance was made from: %v", err)
	}
}

// docs/Incus.md, "Network leases": an instance's lease is its project's to list.
func TestLeasesAreListedPerProject(t *testing.T) {
	r := harness.Get(t)
	if r.Network == "" {
		t.Skip("the default profile's NIC names no managed network")
	}

	name := r.Container(t, harness.ContainerOptions{Running: true})

	leased := func(leases []api.NetworkLease) bool {
		for _, lease := range leases {
			if lease.Hostname == name && lease.Type == "dynamic" {
				return true
			}
		}

		return false
	}

	harness.WaitFor(t, time.Minute, "a dynamic lease in the run's project", func() (bool, string) {
		leases, err := r.Client.GetNetworkLeases(r.Network)
		if err != nil {
			return false, err.Error()
		}

		return leased(leases), ""
	})

	leases, err := r.Default.GetNetworkLeases(r.Network)
	if err != nil {
		t.Fatal(err)
	}

	if leased(leases) {
		t.Fatal("the default project lists another project's lease")
	}
}

// docs/Incus.md, "Volume usage": a dir pool answers with nothing, not an error.
func TestDirPoolReportsNoVolumeUsage(t *testing.T) {
	r := harness.Get(t)

	pool, _, err := r.Client.GetStoragePool(r.Pool)
	if err != nil {
		t.Fatal(err)
	}

	if pool.Driver != "dir" {
		t.Skipf("pool %s is %s, not dir", r.Pool, pool.Driver)
	}

	volume := r.Volume(t)
	name := r.Container(t, harness.ContainerOptions{Running: true})
	r.Attach(t, name, volume, "/mnt/data")
	r.Exec(t, name, "dd", "if=/dev/zero", "of=/mnt/data/fill", "bs=1M", "count=4")

	state, err := r.Client.GetStoragePoolVolumeState(r.Pool, "custom", volume)
	if err != nil {
		t.Fatalf("a dir volume's state is an error, not an empty answer: %v", err)
	}

	if state.Usage != nil && state.Usage.Used != 0 {
		t.Errorf("a dir volume with 4MiB in it reports %d bytes used; docs/Incus.md says it can't", state.Usage.Used)
	}

	instance, _, err := r.Client.GetInstanceState(name)
	if err != nil {
		t.Fatal(err)
	}

	for device, disk := range instance.Disk {
		if disk.Usage != -1 {
			t.Errorf("disk %s reports usage %d, want -1", device, disk.Usage)
		}
	}
}
