package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// beaconModel runs web on nginx:1.27, which incus-beacon gives webStatus
// with 1.31 as the latest, and db, which it says is current, from a
// compose file on disk.
func beaconModel(t *testing.T, webStatus string) (*Model, string, *[]*exec.Cmd) {
	t.Helper()

	return beaconModelLatest(t, webStatus, "1.31")
}

func beaconModelLatest(t *testing.T, webStatus, latest string) (*Model, string, *[]*exec.Cmd) {
	t.Helper()

	stack := testStack(t, t.TempDir(), "default", "web", "db")
	stack.Services[0].Image, stack.Services[1].Image = "nginx:1.27", "postgres:16"

	file := filepath.Join(stack.Dir, "compose.yaml")
	require.NoError(t, os.WriteFile(file, []byte("services:\n  web:\n    image: nginx:1.27\n  db:\n    image: postgres:16\n"), 0o644))

	web := composeFixture("default", "web-1", "web")
	web.ExpandedConfig["user.incus-beacon.status"] = webStatus
	web.ExpandedConfig["user.incus-beacon.current"] = "docker.io/library/nginx:1.27"
	web.ExpandedConfig["user.incus-beacon.latest"] = latest
	db := composeFixture("default", "db-1", "db")
	db.ExpandedConfig["user.incus-beacon.status"] = "current"
	db.Config = map[string]string{"volatile.base_image": "db-image"}

	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{web, db}})
	m := start(t, options{
		compose: true, server: server, localStack: stack,
		configure: func(c *config.UserConfig) { c.Gui.ServiceColumns = []string{"name", "status", "update"} },
	}, 140, 40)

	var ran []*exec.Cmd

	m.runOnTerminal = finishing(&ran)

	return m, file, &ran
}

// The update column says what incus-beacon found, and the Info tab says
// since when.
func TestBeaconVerdictShows(t *testing.T) {
	m, _, _ := beaconModel(t, "update-available")

	press(m, "2")
	text := strings.Join(screen(m), "\n")
	assert.Contains(t, text, "│db  running         ")
	assert.Contains(t, text, "│web running ↑1.31")

	press(m, "j")
	assert.Contains(t, strings.Join(screen(m), "\n"), "Newer tag:    1.31 (incus-beacon)")
}

// G moves the ticked services to their newer tag in the compose file, and
// recreates those alone.
func TestUpdateImagesMovesTagsAndRecreates(t *testing.T) {
	m, file, ran := beaconModel(t, "update-available")

	press(m, "2", "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, []string{"web nginx:1.27 → nginx:1.31"}, menuLabels(m))

	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "move web to nginx:1.31, then pull the newer images of web and recreate them")

	press(m, "y")
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "services:\n  web:\n    image: nginx:1.31\n  db:\n    image: postgres:16\n", string(data))

	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach", "web"}, (*ran)[0].Args[len((*ran)[0].Args)-7:])
}

// A file that no longer names the image is left alone, and nothing runs.
// A newer build and a newer tag are one list, and one recreate on a pull.
func TestUpdateImagesTakesBuildsAndTagsTogether(t *testing.T) {
	m, _, ran := beaconModel(t, "update-available")
	m.upstreams = map[string]commands.UpstreamStatus{"db-image": {
		State: commands.UpstreamNewer, Ref: "docker.io/library/postgres:16",
		From: &commands.BuildInfo{Version: "16.1"}, To: &commands.BuildInfo{Version: "16.2"},
	}}

	press(m, "2", "G")
	assert.Equal(t, []string{"db docker.io/library/postgres:16 16.1 → 16.2", "web nginx:1.27 → nginx:1.31"}, menuLabels(m))

	press(m, "enter", "y")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"--no-deps", "--detach", "db", "web"}, (*ran)[0].Args[len((*ran)[0].Args)-4:])
}

// A rebuild only incus-beacon has seen is G's to take too.
func TestUpdateImagesTakesBeaconsRebuild(t *testing.T) {
	m, _, _ := beaconModelLatest(t, "update-available", "1.27@sha256:0123456789ab")

	press(m, "2", "G")
	assert.Equal(t, []string{"web docker.io/library/nginx:1.27 newer build (incus-beacon)"}, menuLabels(m))
}

func TestUpdateImagesLeavesAnEditedFile(t *testing.T) {
	m, file, ran := beaconModel(t, "update-available")
	require.NoError(t, os.WriteFile(file, []byte("services:\n  web:\n    image: nginx:${TAG}\n"), 0o644))

	press(m, "2", "G", "enter", "y")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "The compose file was left as it was")
	assert.Empty(t, *ran)
}

func TestUpdateImagesWithNoNewerTag(t *testing.T) {
	m, _, _ := beaconModel(t, "current")

	press(m, "2", "G")
	assert.Nil(t, m.topMenu())
	assert.Contains(t, footer(m), "no service of default has a newer image build or tag")
}
