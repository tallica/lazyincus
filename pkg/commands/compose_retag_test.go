package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func beaconService(image, current, status, latest string) *ComposeService {
	return &ComposeService{Name: "web", Image: image, Instances: []*Instance{beaconInstance("web-1", map[string]string{
		beaconStatusKey: status, beaconCurrentKey: current, beaconLatestKey: latest,
	})}}
}

// A bump is offered only for a newer tag of the very image the file names.
func TestComposeServiceTagBump(t *testing.T) {
	bump, ok := beaconService("nginx:1.27", "docker.io/library/nginx:1.27", BeaconUpdateAvailable, "1.31").TagBump()
	require.True(t, ok)
	assert.Equal(t, ComposeTagBump{Service: "web", From: "nginx:1.27", To: "nginx:1.31"}, bump)

	bump, ok = beaconService("ghcr.io/acme/api:2.0.1-alpine", "ghcr.io/acme/api:2.0.1-alpine", BeaconUpdateAvailable, "2.1.0-alpine").TagBump()
	require.True(t, ok)
	assert.Equal(t, "ghcr.io/acme/api:2.1.0-alpine", bump.To)

	refused := map[string]*ComposeService{
		"a rebuild":          beaconService("nginx:1.27", "docker.io/library/nginx:1.27", BeaconUpdateAvailable, "1.27@sha256:0123456789ab"),
		"current":            beaconService("nginx:1.27", "docker.io/library/nginx:1.27", BeaconCurrent, "1.31"),
		"another image":      beaconService("nginx:1.27", "docker.io/library/caddy:1.27", BeaconUpdateAvailable, "1.31"),
		"another tag":        beaconService("nginx:1.27", "docker.io/library/nginx:1.26", BeaconUpdateAvailable, "1.31"),
		"no tag":             beaconService("nginx", "docker.io/library/nginx:latest", BeaconUpdateAvailable, "1.31"),
		"pinned to a digest": beaconService("nginx@sha256:"+sixtyFourHex, "docker.io/library/nginx:1.27", BeaconUpdateAvailable, "1.31"),
		"YAML for a tag":     beaconService("nginx:1.27", "docker.io/library/nginx:1.27", BeaconUpdateAvailable, "1.31\n    privileged: true"),
		"a comment":          beaconService("nginx:1.27", "docker.io/library/nginx:1.27", BeaconUpdateAvailable, "1.31 # x"),
	}
	for name, service := range refused {
		_, ok := service.TagBump()
		assert.False(t, ok, name)
	}
}

const sixtyFourHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// Only the image values move: comments, quoting and every other line stay.
func TestRetagComposeImages(t *testing.T) {
	file := filepath.Join(t.TempDir(), "compose.yaml")
	original := `name: shop
services:
  # the front end
  web:
    image: "nginx:1.27"   # pinned
    ports: ["8080:80"]
  api:
    image: ghcr.io/acme/api:2.0.1-alpine
  db:
    image: postgres:16
`
	require.NoError(t, os.WriteFile(file, []byte(original), 0o640))

	require.NoError(t, RetagComposeImages(file, []ComposeTagBump{
		{Service: "web", From: "nginx:1.27", To: "nginx:1.31"},
		{Service: "api", From: "ghcr.io/acme/api:2.0.1-alpine", To: "ghcr.io/acme/api:2.1.0-alpine"},
	}))

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, `name: shop
services:
  # the front end
  web:
    image: "nginx:1.31"   # pinned
    ports: ["8080:80"]
  api:
    image: ghcr.io/acme/api:2.1.0-alpine
  db:
    image: postgres:16
`, string(data))

	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

// A file that doesn't name the image as the bump expects is left alone,
// whichever bump fails.
func TestRetagComposeImagesRefuses(t *testing.T) {
	file := filepath.Join(t.TempDir(), "compose.yaml")
	original := "services:\n  web:\n    image: nginx:1.27\n  api:\n    image: ghcr.io/acme/api:${TAG}\n  worker:\n    extends: api\n"
	require.NoError(t, os.WriteFile(file, []byte(original), 0o644))

	web := ComposeTagBump{Service: "web", From: "nginx:1.27", To: "nginx:1.31"}

	for name, bump := range map[string]ComposeTagBump{
		"a variable":        {Service: "api", From: "ghcr.io/acme/api:2.0", To: "ghcr.io/acme/api:2.1"},
		"no image line":     {Service: "worker", From: "ghcr.io/acme/api:2.0", To: "ghcr.io/acme/api:2.1"},
		"no such service":   {Service: "cache", From: "redis:7", To: "redis:8"},
		"an image moved on": {Service: "web", From: "nginx:1.26", To: "nginx:1.31"},
	} {
		assert.Error(t, RetagComposeImages(file, []ComposeTagBump{web, bump}), name)

		data, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, original, string(data), name)
	}
}

// A symlinked compose file is edited where it points, the link kept.
func TestRetagComposeImagesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(target, []byte("services:\n  web:\n    image: nginx:1.27\n"), 0o600))
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, RetagComposeImages(link, []ComposeTagBump{{Service: "web", From: "nginx:1.27", To: "nginx:1.31"}}))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)

	info, err = os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temp file left")
}
