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
}
