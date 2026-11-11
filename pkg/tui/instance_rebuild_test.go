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

// U offers the newest build of the tag and beacon's newer tag; the rebuild
// stops the instance, and starts it again after.
func TestRebuildingAnOCIContainer(t *testing.T) {
	server := ociServer()
	m := start(t, options{server: server}, 140, 40)

	press(m, "U")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, []string{
		"from the newest build of docker.io/library/nginx:1.27",
		"onto docker.io/library/nginx:1.31, the newer tag incus-beacon found",
	}, itemLabels(m)[:2])

	press(m, "j", "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "rebuild web from docker.io/library/nginx:1.31? Its root disk is replaced")

	press(m, "y")
	assert.Equal(t, []string{"web=library/nginx:1.31"}, server.Rebuilds())

	instances, err := server.GetInstancesFull(api.InstanceTypeAny)
	require.NoError(t, err)
	assert.Equal(t, "Running", instances[0].Status)
}

// incusd won't rebuild an instance with snapshots, so U says so up front.
func TestRebuildingAnInstanceWithSnapshots(t *testing.T) {
	server := ociServer()
	_, err := server.CreateInstanceSnapshot("web", api.InstanceSnapshotsPost{Name: "snap0"})
	require.NoError(t, err)
	m := start(t, options{server: server}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "it has snapshots")
	assert.Empty(t, server.Rebuilds())
}

func TestRebuildingNeedsAnOCIContainer(t *testing.T) {
	m := start(t, options{server: instancesServer(nil, runningInstance("web", nil))}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	assert.Contains(t, footer(m), "only an OCI container can be rebuilt from its image")
}

// An image deleted since leaves nothing to say which registry to pull from.
func TestRebuildingWithTheImageGone(t *testing.T) {
	server := ociServer()
	server.SetImages(nil)
	m := start(t, options{server: server}, 140, 40)

	press(m, "U")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "web can't be rebuilt")
}
