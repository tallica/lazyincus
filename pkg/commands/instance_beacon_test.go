package commands

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func beaconInstance(name string, config map[string]string) *Instance {
	return &Instance{Name: name, Instance: api.InstanceFull{Instance: api.Instance{Name: name, ExpandedConfig: config}}}
}

func TestInstanceBeacon(t *testing.T) {
	instance := beaconInstance("web", map[string]string{
		beaconStatusKey: BeaconUpdateAvailable,
		beaconLatestKey: "1.31",
		beaconSinceKey:  "2026-11-10T09:12:00Z",
	})

	verdict := instance.Beacon()
	assert.Equal(t, BeaconVerdict{
		Status: BeaconUpdateAvailable, Latest: "1.31", Since: time.Date(2026, 11, 10, 9, 12, 0, 0, time.UTC),
	}, verdict)
	assert.False(t, verdict.Rebuilt())
	assert.Equal(t, "1.31", verdict.NewerTag())

	rebuilt := BeaconVerdict{Status: BeaconUpdateAvailable, Latest: "1.27@sha256:0123456789ab"}
	assert.True(t, rebuilt.Rebuilt())
	assert.Empty(t, rebuilt.NewerTag(), "a rebuild is a pull, not a new tag")

	assert.Empty(t, BeaconVerdict{Status: BeaconCurrent, Latest: "1.31"}.NewerTag())
	assert.Equal(t, BeaconVerdict{}, beaconInstance("db", nil).Beacon())
}

// A service says what its most telling instance does: an update over an
// error over unknown over current.
func TestComposeServiceBeacon(t *testing.T) {
	status := func(name, status string) *Instance {
		return beaconInstance(name, map[string]string{beaconStatusKey: status})
	}

	service := &ComposeService{Instances: []*Instance{status("web-1", BeaconCurrent), status("web-2", BeaconError), status("web-3", "")}}
	assert.Equal(t, BeaconError, service.Beacon().Status)

	service.Instances = append(service.Instances, status("web-4", BeaconUpdateAvailable))
	assert.Equal(t, BeaconUpdateAvailable, service.Beacon().Status)

	assert.Empty(t, (&ComposeService{}).Beacon().Status)

	rebuilt := beaconInstance("web-5", map[string]string{beaconStatusKey: BeaconUpdateAvailable, beaconLatestKey: "1.27@sha256:0123456789ab"})
	tagged := beaconInstance("web-6", map[string]string{beaconStatusKey: BeaconUpdateAvailable, beaconLatestKey: "1.31"})
	service.Instances = []*Instance{rebuilt, tagged}
	assert.Equal(t, "1.31", service.Beacon().Latest, "a newer tag over a rebuild, whichever replica comes first")
}

func TestBeaconCurrentTag(t *testing.T) {
	for current, want := range map[string]string{
		"docker.io/library/nginx:1.18-alpine": "1.18-alpine",
		"registry.example:5000/nginx:1.18":    "1.18",
		"registry.example:5000/nginx":         "latest",
		"":                                    "",
	} {
		assert.Equal(t, want, BeaconVerdict{Current: current}.CurrentTag(), current)
	}
}

func TestBeaconSemver(t *testing.T) {
	assert.False(t, beaconVerdict(map[string]string{}).Semver)
	assert.True(t, beaconVerdict(map[string]string{beaconTrackLabel: "semver"}).Semver)
	assert.False(t, beaconVerdict(map[string]string{beaconTrackKey: "digest", beaconTrackLabel: "semver"}).Semver,
		"the instance's own key wins")
}

// A verdict about the tag an instance ran before a rebuild is no verdict.
func TestBeaconVerdictOutrun(t *testing.T) {
	config := map[string]string{
		beaconStatusKey: BeaconUpdateAvailable, beaconCurrentKey: "docker.io/library/nginx:1.27-alpine", beaconLatestKey: "1.28-alpine",
		"image.id": "library/nginx:1.28-alpine",
	}
	assert.Equal(t, BeaconVerdict{}, beaconVerdict(config))

	config["image.id"] = "library/nginx:1.27-alpine"
	assert.Equal(t, BeaconUpdateAvailable, beaconVerdict(config).Status)

	config["image.id"] = "library/nginx@sha256:0123"
	assert.Equal(t, BeaconUpdateAvailable, beaconVerdict(config).Status, "a digest names no tag to compare")

	config["user.image_alias"] = "docker.io/library/nginx:1.28-alpine"
	assert.Equal(t, BeaconVerdict{}, beaconVerdict(config), "incus-compose's alias says what it runs")

	config[beaconCurrentKey] = "docker.io/library/nginx:latest"
	config["user.image_alias"] = "nginx"
	assert.Equal(t, BeaconUpdateAvailable, beaconVerdict(config).Status, "an untagged image runs latest")

	config["user.image_alias"] = "nginx:1.27"
	assert.Equal(t, BeaconVerdict{}, beaconVerdict(config))

	config = map[string]string{
		beaconStatusKey: BeaconCurrent, beaconCurrentKey: "docker.io/library/nginx:1.31-alpine",
		"image.id": "library/nginx:alpine", beaconImageKey: "docker.io/library/nginx:1.31-alpine",
	}
	assert.Equal(t, BeaconCurrent, beaconVerdict(config).Status, "beacon's image key over the tag Incus kept")
}

func TestInstanceTracksAnotherImage(t *testing.T) {
	tracks := func(tracked string) bool {
		return beaconInstance("web", map[string]string{
			imageDescriptionKey: "docker.io/library/nginx (OCI)", beaconImageKey: tracked,
		}).TracksAnotherImage()
	}

	assert.False(t, tracks(""))
	assert.False(t, tracks("docker.io/library/nginx:1.31"))
	assert.False(t, tracks("nginx:1.31"), "Docker's short name")
	assert.True(t, tracks("docker.io/library/caddy:2"))
	assert.True(t, tracks("registry.example/library/nginx:1.31"), "a mirror is another image")
}
