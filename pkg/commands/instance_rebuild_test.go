package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOCIRebuildRefs(t *testing.T) {
	source := OCIRebuild{Server: "https://registry.example:5000", Ref: "acme/api:2.0"}

	assert.Equal(t, "registry.example:5000/acme/api:2.0", source.Image())
	assert.Equal(t, "registry.example:5000/acme/api:2.1", source.WithTag("2.1").Image(), "the port isn't a tag")
}

// Beacon's tag is offered only for the very repository the instance runs,
// and only when it is a tag.
func TestBeaconNewerTagForARebuild(t *testing.T) {
	source := OCIRebuild{Server: "https://docker.io", Ref: "library/nginx:1.27"}
	instance := func(current, latest string) *Instance {
		return beaconInstance("web", map[string]string{
			beaconStatusKey: BeaconUpdateAvailable, beaconCurrentKey: current, beaconLatestKey: latest,
		})
	}

	assert.Equal(t, "1.31", instance("docker.io/library/nginx:1.27", "1.31").BeaconNewerTag(source))
	assert.Empty(t, instance("docker.io/library/caddy:1.27", "1.31").BeaconNewerTag(source), "another image")
	assert.Empty(t, instance("docker.io/library/nginx:1.27", "1.31\nx: y").BeaconNewerTag(source), "not a tag")
	assert.Empty(t, instance("docker.io/library/nginx:1.27", "1.27@sha256:0123456789ab").BeaconNewerTag(source), "a rebuild")
}
