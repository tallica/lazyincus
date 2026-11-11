package commands

import (
	"strings"
	"time"
)

// The keys incus-beacon writes: docs/Panels.md, "incus-beacon".
const (
	beaconStatusKey  = "user.incus-beacon.status"
	beaconCurrentKey = "user.incus-beacon.current"
	beaconLatestKey  = "user.incus-beacon.latest"
	beaconErrorKey   = "user.incus-beacon.error"
	beaconSinceKey   = "user.incus-beacon.since"
)

const (
	BeaconCurrent         = "current"
	BeaconUpdateAvailable = "update-available"
	BeaconError           = "error"
	BeaconUnknown         = "unknown"
)

// BeaconVerdict is incus-beacon's last check of an image, zero for none.
type BeaconVerdict struct {
	Status string
	// Latest is the newer tag, or <tag>@sha256:<12 hex> for the same tag rebuilt.
	Latest string
	// Current is the image beacon checked, tag and all.
	Current string
	Error   string
	Since   time.Time
}

// Rebuilt is a newer build of the same tag rather than a newer tag.
func (v BeaconVerdict) Rebuilt() bool {
	return strings.Contains(v.Latest, "@sha256:")
}

// NewerTag is the tag beacon found, empty unless it found one.
func (v BeaconVerdict) NewerTag() string {
	if v.Status != BeaconUpdateAvailable || v.Rebuilt() {
		return ""
	}

	return v.Latest
}

// CurrentTag is Current's tag, empty when it names none.
func (v BeaconVerdict) CurrentTag() string {
	name := v.Current[strings.LastIndex(v.Current, "/")+1:]
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}

	return ""
}

func (i *Instance) Beacon() BeaconVerdict {
	return beaconVerdict(i.Instance.ExpandedConfig)
}

func beaconVerdict(config map[string]string) BeaconVerdict {
	verdict := BeaconVerdict{
		Status:  config[beaconStatusKey],
		Latest:  config[beaconLatestKey],
		Current: config[beaconCurrentKey],
		Error:   config[beaconErrorKey],
	}

	if since, err := time.Parse(time.RFC3339, config[beaconSinceKey]); err == nil {
		verdict.Since = since
	}

	return verdict
}

// Beacon is the most telling of its instances' verdicts.
func (s *ComposeService) Beacon() BeaconVerdict {
	var best BeaconVerdict

	for _, instance := range s.SortedInstances() {
		best = moreTelling(best, instance.Beacon())
	}

	return best
}

// moreTelling ranks an update over an error over unknown over current.
func moreTelling(a, b BeaconVerdict) BeaconVerdict {
	rank := map[string]int{BeaconCurrent: 1, BeaconUnknown: 2, BeaconError: 3, BeaconUpdateAvailable: 4}
	if rank[b.Status] > rank[a.Status] {
		return b
	}

	return a
}
