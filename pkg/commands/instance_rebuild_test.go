package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOCIRebuildRefs(t *testing.T) {
	source := OCIRebuild{Server: "https://registry.example:5000", Ref: "acme/api:2.0"}

	assert.Equal(t, "registry.example:5000/acme/api:2.0", source.Image())
	assert.Equal(t, "registry.example:5000/acme/api:2.1", source.WithTag("2.1").Image(), "the port isn't a tag")

	source.Server = "https://ghcr.io/"
	assert.Equal(t, "ghcr.io/acme/api:2.0", source.Image())
}

// Beacon's tag is offered only for the very repository the instance runs,
// and only when it is a tag.
func TestBeaconNewerTagFor(t *testing.T) {
	source := OCIRebuild{Server: "https://docker.io", Ref: "library/nginx:1.27"}
	verdict := func(current, latest string) BeaconVerdict {
		return BeaconVerdict{Status: BeaconUpdateAvailable, Current: current, Latest: latest}
	}

	assert.Equal(t, "1.31", verdict("docker.io/library/nginx:1.27", "1.31").NewerTagFor(source))
	assert.Empty(t, verdict("docker.io/library/caddy:1.27", "1.31").NewerTagFor(source), "another image")
	assert.Empty(t, verdict("docker.io/library/nginx:1.27", "1.31\nx: y").NewerTagFor(source), "not a tag")
	assert.Empty(t, verdict("docker.io/library/nginx:1.27", "1.27@sha256:0123456789ab").NewerTagFor(source), "a rebuild")

	short := OCIRebuild{Server: "https://docker.io", Ref: "nginx:1.27"}
	assert.Equal(t, "1.31", verdict("docker.io/library/nginx:1.27", "1.31").NewerTagFor(short), "Incus may keep the short name")
}

// An image Incus has expired leaves the keys it copied onto the instance.
func TestOCISourceFromConfig(t *testing.T) {
	source, ok := ociSourceFromConfig("docker.io/semaphoreui/semaphore (OCI)", "semaphoreui/semaphore:latest")
	assert.True(t, ok)
	assert.Equal(t, OCIRebuild{Server: "https://docker.io", Ref: "semaphoreui/semaphore:latest"}, source)

	source, ok = ociSourceFromConfig("registry.example:5000/acme/api (OCI)", "acme/api:2.0")
	assert.True(t, ok)
	assert.Equal(t, "registry.example:5000/acme/api:2.0", source.Image())

	for name, keys := range map[string][2]string{
		"no description":   {"", "library/nginx:1.27"},
		"no tag":           {"docker.io/library/nginx (OCI)", "library/nginx"},
		"another image":    {"docker.io/library/caddy (OCI)", "library/nginx:1.27"},
		"not an OCI image": {"Alpine 3.22 amd64 (20261111_13:00)", "alpine/3.22"},
	} {
		_, ok := ociSourceFromConfig(keys[0], keys[1])
		assert.False(t, ok, name)
	}
}
