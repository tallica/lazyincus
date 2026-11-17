package harness

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// image is small, and a system container rather than OCI, so it runs on every lane.
var image = api.InstanceSource{
	Type:     "image",
	Mode:     "pull",
	Server:   "https://images.linuxcontainers.org",
	Protocol: "simplestreams",
	Alias:    "alpine/3.22",
}

// ContainerOptions default to a stopped container on the default profile.
type ContainerOptions struct {
	Running  bool
	Profiles []string
}

// Container creates a container in the run's project, deleted when t ends.
func (r *Run) Container(t *testing.T, opts ContainerOptions) string {
	t.Helper()

	name := "c-" + Suffix()
	profiles := append([]string{"default"}, opts.Profiles...)

	op, err := r.Client.CreateInstance(api.InstancesPost{
		Name:   name,
		Type:   api.InstanceTypeContainer,
		Source: image,
		InstancePut: api.InstancePut{
			Profiles: profiles,
		},
	})
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}

	t.Cleanup(func() { r.removeInstance(t, name) })

	if opts.Running {
		r.SetState(t, name, "start")
	}

	return name
}

func (r *Run) removeInstance(t *testing.T, name string) {
	t.Helper()

	stop, err := r.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: "stop", Force: true, Timeout: -1}, "")
	if err == nil {
		_ = stop.Wait()
	}

	del, err := r.Client.DeleteInstance(name)
	if err == nil {
		err = del.Wait()
	}

	if err != nil && !api.StatusErrorCheck(err, http.StatusNotFound) {
		t.Errorf("deleting %s: %v", name, err)
	}
}

// SetState changes an instance's state behind lazyincus's back, as the CLI would.
func (r *Run) SetState(t *testing.T, name, action string) {
	t.Helper()

	op, err := r.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: action, Timeout: -1}, "")
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatalf("%s %s: %v", action, name, err)
	}
}

func (r *Run) Exec(t *testing.T, name string, command ...string) {
	t.Helper()

	op, err := r.Client.ExecInstance(name, api.InstanceExecPost{Command: command}, &incus.InstanceExecArgs{})
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatalf("exec %v in %s: %v", command, name, err)
	}
}

// Volume creates a custom volume on the run's pool, deleted when t ends.
func (r *Run) Volume(t *testing.T) string {
	t.Helper()

	name := "v-" + Suffix()

	if err := r.Client.CreateStoragePoolVolume(r.Pool, api.StorageVolumesPost{Name: name, Type: "custom"}); err != nil {
		t.Fatalf("creating volume %s: %v", name, err)
	}

	t.Cleanup(func() {
		err := r.Client.DeleteStoragePoolVolume(r.Pool, "custom", name)
		if err != nil && !api.StatusErrorCheck(err, http.StatusNotFound) {
			t.Errorf("deleting volume %s: %v", name, err)
		}
	})

	return name
}

func (r *Run) Attach(t *testing.T, instance, volume, path string) {
	t.Helper()

	full, etag, err := r.Client.GetInstance(instance)
	if err != nil {
		t.Fatal(err)
	}

	put := full.Writable()
	if put.Devices == nil {
		put.Devices = map[string]map[string]string{}
	}

	put.Devices[volume] = map[string]string{"type": "disk", "pool": r.Pool, "source": volume, "path": path}

	op, err := r.Client.UpdateInstance(instance, put, etag)
	if err == nil {
		err = op.Wait()
	}

	if err != nil {
		t.Fatalf("attaching %s to %s: %v", volume, instance, err)
	}
}

// Instance is the named instance as lazyincus lists it.
func (r *Run) Instance(t *testing.T, name string) *commands.Instance {
	t.Helper()

	instances, err := r.Command.GetProjectInstances(r.Project, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, instance := range instances {
		if instance.Name == name {
			return instance
		}
	}

	t.Fatalf("%s isn't listed in %s", name, r.Project)

	return nil
}

// Events records the run's project's events as lazyincus's stream reads them.
type Events struct {
	mutex  sync.Mutex
	events []commands.Event
}

// Listen starts recording, open by the time it returns, until t ends.
func (r *Run) Listen(t *testing.T) *Events {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	opened := make(chan struct{})
	stopped := make(chan error, 1)
	recorded := &Events{}

	go func() {
		stopped <- r.Command.ListenForEventsIn(ctx, r.Project, []string{api.EventTypeLifecycle, api.EventTypeOperation},
			func() { close(opened) },
			func(event commands.Event) {
				recorded.mutex.Lock()
				recorded.events = append(recorded.events, event)
				recorded.mutex.Unlock()
			})
	}()

	t.Cleanup(cancel)

	select {
	case <-opened:
	case err := <-stopped:
		t.Fatalf("event stream: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("event stream didn't open within 10s")
	}

	return recorded
}

func (e *Events) All() []commands.Event {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return slices.Clone(e.events)
}

func (e *Events) Since(n int) []commands.Event {
	return e.All()[n:]
}

func (e *Events) Has(match func(commands.Event) bool) bool {
	return slices.ContainsFunc(e.All(), match)
}

func (e *Events) Actions() string {
	all := e.All()
	actions := make([]string, len(all))

	for i, event := range all {
		actions[i] = event.Action
	}

	return strings.Join(actions, ", ")
}
