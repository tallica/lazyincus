package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// build pulled first - and beacon's newer tag; recreating stops the
// instance, and starts it again after.
func TestRecreatingAnOCIContainer(t *testing.T) {
	for choice, want := range map[int]string{0: "web=nginx-image", 1: "web=library/nginx:1.27", 2: "web=library/nginx:1.31"} {
		server := ociServer()
		m := start(t, options{server: server}, 140, 40)

		press(m, "U")
		require.NotNil(t, m.topMenu())
		assert.Equal(t, []string{
			"with the current image",
			"pull the latest build of docker.io/library/nginx:1.27 first",
			"onto docker.io/library/nginx:1.31, the newer tag incus-beacon found",
		}, itemLabels(m)[:3])

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
	assert.Equal(t, []string{
		"pull the latest build of docker.io/library/nginx:1.27 first",
		"onto docker.io/library/nginx:1.31, the newer tag incus-beacon found",
	}, itemLabels(m)[:2])

	press(m, "enter", "y")
	assert.Equal(t, []string{"web=library/nginx:1.27"}, server.Rebuilds())
}
