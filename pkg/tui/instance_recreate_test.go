package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// ociServer runs web, an OCI container pulled as nginx:1.27, which
// incus-beacon says has 1.31 out.
func ociServer() *incustest.Server {
	web := runningInstance("web", map[string]string{
		"volatile.container.oci":    "true",
		"user.incus-beacon.status":  "update-available",
		"user.incus-beacon.current": "docker.io/library/nginx:1.27",
		"user.incus-beacon.latest":  "1.31",
	})
	web.Config = map[string]string{"volatile.base_image": "nginx-image"}

	return incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{web}, Images: []api.Image{{
		Fingerprint: "nginx-image", Project: "default",
		UpdateSource: &api.ImageSource{Protocol: "oci", Server: "https://docker.io", Alias: "library/nginx:1.27@sha256:0123"},
	}}})
}

func itemLabels(m *Model) []string {
	labels := make([]string, 0, len(m.topMenu().items))
	for _, item := range m.topMenu().items {
		labels = append(labels, item.label)
	}

	return labels
}

// U offers what U on Services does - the current image, or the latest
// build pulled first; recreating stops the instance, and starts it again
// after.
func TestRecreatingAnOCIContainer(t *testing.T) {
	for choice, want := range map[int]string{0: "web=nginx-image", 1: "web=library/nginx:1.27"} {
		server := ociServer()
		m := start(t, options{server: server}, 140, 40)

		press(m, "U")
		require.NotNil(t, m.topMenu())
		assert.Equal(t, []string{
			"recreate",
			"pull & recreate",
		}, itemLabels(m)[:2])

		for range choice {
			press(m, "j")
		}

		press(m, "enter")
		require.Len(t, m.popups, 1)
		assert.Contains(t, ansi.Strip(m.popups[0].message), "Are you sure you want to recreate web")

		press(m, "y")
		assert.Equal(t, []string{want}, server.Rebuilds())

		instances, err := server.GetInstancesFull(api.InstanceTypeAny)
		require.NoError(t, err)
		assert.Equal(t, "Running", instances[0].Status)
	}
}

// incusd won't rebuild an instance with snapshots, so U says so up front.
func TestRecreatingAnInstanceWithSnapshots(t *testing.T) {
	server := ociServer()
	_, err := server.CreateInstanceSnapshot("web", api.InstanceSnapshotsPost{Name: "snap0"})
	require.NoError(t, err)
	m := start(t, options{server: server}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "Incus only recreates an instance without snapshots, and web has 1: delete them")
	assert.Empty(t, server.Rebuilds())
}

func TestRecreatingNeedsAnOCIContainer(t *testing.T) {
	m := start(t, options{server: instancesServer(nil, runningInstance("web", nil))}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	assert.Contains(t, footer(m), "only an OCI container can be recreated from its image")
}

// An image Incus has expired since can't be recreated from as it is, but
// the instance's own keys still say where to pull it from; with neither,
// there's nothing to go on.
func TestRecreatingWithTheImageGone(t *testing.T) {
	server := ociServer()
	server.SetImages(nil)
	m := start(t, options{server: server}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "web can't be recreated")

	instances, err := server.GetInstancesFull(api.InstanceTypeAny)
	require.NoError(t, err)
	web := instances[0]
	web.ExpandedConfig["image.description"] = "docker.io/library/nginx (OCI)"
	web.ExpandedConfig["image.id"] = "library/nginx:1.27"
	server = incustest.New(incustest.Server{Version: "7.4", Instances: []api.InstanceFull{web}})
	m = start(t, options{server: server}, 140, 40)

	press(m, "U")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, "pull & recreate", itemLabels(m)[0])

	press(m, "enter", "y")
	assert.Equal(t, []string{"web=library/nginx:1.27"}, server.Rebuilds())
}

// G lists the OCI containers of no stack with something newer, beacon's
// tag as from -> to, and recreates the ticked ones onto it.
func TestUpdatingOCIContainers(t *testing.T) {
	server := ociServer()
	m := start(t, options{server: server}, 140, 40)

	press(m, "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, []string{"web docker.io/library/nginx:1.27 → 1.31"}, menuLabels(m))

	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "recreate web from docker.io/library/nginx:1.31?")

	press(m, "y")
	assert.Equal(t, []string{"web=library/nginx:1.31"}, server.Rebuilds())
}

// An instance with snapshots and something newer is named, not dropped:
// alone it's the error, beside others the checklist's subtitle.
func TestUpdatingOCIContainersWithSnapshots(t *testing.T) {
	server := ociServer()
	_, err := server.CreateInstanceSnapshot("web", api.InstanceSnapshotsPost{Name: "snap0"})
	require.NoError(t, err)
	m := start(t, options{server: server}, 140, 40)

	press(m, "G")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "every one here with something newer has some (web)")

	server = twoOCIServer(nil)
	_, err = server.CreateInstanceSnapshot("api", api.InstanceSnapshotsPost{Name: "snap0"})
	require.NoError(t, err)
	m = start(t, options{server: server}, 140, 40)

	press(m, "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, []string{"web docker.io/library/nginx:1.27 → 1.31"}, menuLabels(m))
	assert.Equal(t, "left out, with snapshots: api", m.topMenu().subtitle)
}

// twoOCIServer adds api, a second OCI container beacon has a newer tag for.
func twoOCIServer(rebuildErrors map[string]error) *incustest.Server {
	web := ociServer().Instances[0]
	other := runningInstance("api", map[string]string{
		"volatile.container.oci":    "true",
		"user.incus-beacon.status":  "update-available",
		"user.incus-beacon.current": "docker.io/acme/api:2.0",
		"user.incus-beacon.latest":  "2.1",
	})
	other.Config = map[string]string{"volatile.base_image": "api-image"}

	return incustest.New(incustest.Server{
		Version: "7.4", Instances: []api.InstanceFull{other, web}, RebuildErrors: rebuildErrors,
		Images: append(ociServer().Images, api.Image{
			Fingerprint: "api-image", Project: "default",
			UpdateSource: &api.ImageSource{Protocol: "oci", Server: "https://docker.io", Alias: "acme/api:2.0"},
		}),
	})
}

// One container failing leaves the others recreated, the error naming it.
func TestUpdatingOCIContainersPastAFailure(t *testing.T) {
	server := twoOCIServer(map[string]error{"api": errors.New("registry said 401")})
	m := start(t, options{server: server}, 140, 40)

	press(m, "G")
	require.Len(t, menuLabels(m), 2)

	press(m, "enter", "y")
	assert.Equal(t, []string{"web=library/nginx:1.31"}, server.Rebuilds())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "api: registry said 401")
}

// With nothing checked and beacon not on every container, G asks the
// registries first, as on Services, and again after the recreate; beacon
// on every one, and it asks nothing.
func TestUpdatingOCIContainersChecksFirst(t *testing.T) {
	for beacon, wantChecks := range map[bool]int{true: 0, false: 2} {
		server := ociServer()
		if !beacon {
			instances := server.Instances
			delete(instances[0].ExpandedConfig, "user.incus-beacon.status")
			server.SetInstances(instances)
		}

		m := start(t, options{server: server}, 140, 40)

		checks := 0
		m.checkUpstreams = func(_ context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
			checks++
			return map[string]commands.UpstreamStatus{"nginx-image": {State: commands.UpstreamNewer, Ref: "docker.io/library/nginx:1.27"}}
		}

		press(m, "G")
		require.NotNil(t, m.topMenu(), "beacon %v", beacon)

		press(m, "enter", "y")
		assert.Equal(t, wantChecks, checks, "beacon %v", beacon)
	}
}

// The Info tab says what g found, as the update column does.
func TestInstanceInfoSaysWhatGFound(t *testing.T) {
	server := ociServer()
	instances := server.Instances
	delete(instances[0].ExpandedConfig, "user.incus-beacon.status")
	server.SetInstances(instances)
	m := start(t, options{server: server}, 140, 40)
	m.checkUpstreams = func(context.Context, []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
		return map[string]commands.UpstreamStatus{"nginx-image": {State: commands.UpstreamNewer, Ref: "docker.io/library/nginx:1.27"}}
	}

	assert.NotContains(t, mainText(m), "Newer build:")
	run(m, refresh(m.imageUpdatesFetch(false)))
	assert.Contains(t, mainText(m), "Newer build:  yes, G takes it")
}

// U and G change the daemon, so a read-only remote refuses both.
func TestRecreatingIsRefusedReadOnly(t *testing.T) {
	server := ociServer()
	m := start(t, options{server: server, readOnly: true}, 140, 40)

	for _, key := range []string{"U", "G"} {
		press(m, key)
		assert.Nil(t, m.topMenu(), key)
		require.Len(t, m.popups, 1, key)
		press(m, "esc")
	}

	assert.Empty(t, server.Rebuilds())
}

// Nothing newer leaves G nothing to list.
func TestUpdatingOCIContainersWithNothingToTake(t *testing.T) {
	server := ociServer()
	instances := server.Instances
	instances[0].ExpandedConfig["user.incus-beacon.status"] = "current"
	server.SetInstances(instances)
	m := start(t, options{server: server}, 140, 40)

	press(m, "G")
	assert.Nil(t, m.topMenu())
	assert.Contains(t, footer(m), "no OCI container here has a newer image build or tag to take")
}
