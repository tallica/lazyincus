package tui

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

func warningStatuses(server *incustest.Server) map[string]string {
	warnings, _ := server.GetWarnings()

	return lo.SliceToMap(warnings, func(warning api.Warning) (string, string) { return warning.UUID, warning.Status })
}

func TestWarningKeys(t *testing.T) {
	server := warningsServer()
	m := start(t, options{server: server}, 140, 40)
	assert.Contains(t, footer(m), "2 warnings")

	press(m, "W")
	require.Equal(t, "disk", m.daemonPopup().menu.selectedItem().key, "new first, the most severe first")

	press(m, "a")
	assert.Equal(t, "acknowledged", warningStatuses(server)["disk"])
	assert.Contains(t, footer(m), "1 warning ")
	require.NotNil(t, m.daemonPopup(), "the popup stays open")
	assert.Equal(t, "disk", m.daemonPopup().menu.selectedItem().key, "on the same warning")

	// d asks first, with the whole warning; no goes back to the list, on it.
	press(m, "d")
	assert.Contains(t, popupText(m), "delete this warning")
	assert.Contains(t, popupText(m), "Storage pool almost full")
	assert.Nil(t, m.daemonPopup(), "a confirmation hides the menu, as pkg/gui's does")
	press(m, "n")
	require.NotNil(t, m.daemonPopup())
	assert.Equal(t, "disk", m.daemonPopup().menu.selectedItem().key)

	press(m, "d", "enter")
	assert.NotContains(t, warningStatuses(server), "disk")
	require.NotNil(t, m.daemonPopup())
	assert.Contains(t, popupText(m), "a: acknowledge, d: delete")
	assert.NotContains(t, popupText(m), "Storage pool almost full")
}

func TestClosingAWarningsDetailsGoesBackToIt(t *testing.T) {
	m := start(t, options{server: warningsServer()}, 140, 40)

	press(m, "W", "down", "enter")
	assert.Contains(t, popupText(m), "First seen:")
	assert.Contains(t, popupText(m), "the server's")
	press(m, "esc")
	require.NotNil(t, m.daemonPopup())
	assert.Equal(t, "kvm", m.daemonPopup().menu.selectedItem().key)
}

func TestDaemonKeysHonourReadOnly(t *testing.T) {
	server := warningsServer()
	server.SetOperations([]api.Operation{liveCancellable()})
	m := start(t, options{server: server, configure: func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}}, 140, 40)

	for _, key := range []string{"a", "d"} {
		press(m, "W", key)
		assert.Contains(t, popupText(m), "fake is read-only")
		press(m, "esc")
	}
	assert.Equal(t, "new", warningStatuses(server)["disk"])

	press(m, "W", "[", "d")
	assert.Contains(t, popupText(m), "fake is read-only")
	assert.Empty(t, server.Cancelled())
}

func liveCancellable() api.Operation {
	return api.Operation{
		ID: "download", Class: "task", Description: "Downloading image",
		Status: "Running", StatusCode: api.Running, MayCancel: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Resources: map[string][]string{"images": {"/1.0/images/0123456789ab"}},
		Metadata:  map[string]any{"download_progress": "rootfs: 23% (5.20MB/s)"},
	}
}

func TestOperationKeys(t *testing.T) {
	server := eventServer()
	server.SetOperations([]api.Operation{liveCancellable()})
	m := start(t, options{server: server}, 140, 40)

	press(m, "W", "enter")
	assert.Contains(t, popupText(m), "Progress:     rootfs: 23% (5.20MB/s)")
	assert.Contains(t, popupText(m), "images 0123456789ab")
	press(m, "esc")
	assert.Contains(t, popupText(m), "d: cancel, enter: details")

	press(m, "d")
	assert.Contains(t, popupText(m), "Are you sure you want to cancel Downloading image 0123456789ab?")
	press(m, "enter")
	assert.Equal(t, []string{"download"}, server.Cancelled())
	require.NotNil(t, m.daemonPopup(), "back to the list")
}

func TestAnEndedOperationIsNotCancelled(t *testing.T) {
	server := eventServer()
	m := start(t, options{server: server}, 140, 40)
	m.recordOperation(testOperation("ended", api.Success, 0))

	press(m, "W", "d")
	assert.Contains(t, popupText(m), "can't be cancelled")
	assert.Empty(t, server.Cancelled())
}

// A websocket's metadata is its connection secrets.
func TestAnOperationsDetailsHideAWebsocketsSecrets(t *testing.T) {
	server := eventServer()
	exec := liveCancellable()
	exec.Class = "websocket"
	exec.Metadata = map[string]any{"fds": map[string]any{"0": "secret-token"}}
	server.SetOperations([]api.Operation{exec})
	m := start(t, options{server: server}, 140, 40)

	press(m, "W", "enter")
	assert.Contains(t, popupText(m), "Class:        websocket")
	assert.NotContains(t, popupText(m), "secret-token")
}

func TestARetriedFailureReadsAsRetried(t *testing.T) {
	m := start(t, options{}, 140, 40)
	failed := testOperation("refused", api.Failure, 0)
	retry := testOperation("deleted", api.Success, 0)
	retry.Operation.CreatedAt = failed.Operation.CreatedAt.Add(time.Second)
	m.recordOperation(failed)
	m.recordOperation(retry)

	press(m, "W")
	failure := lo.Must(lo.Find(m.daemonPopup().menu.items, func(item *menuItem) bool { return item.key == "refused" }))
	assert.Contains(t, failure.labelColumns, presentation.OperationStatus(failed, true))
	assert.NotContains(t, footer(m), "failed")
}
