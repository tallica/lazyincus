package presentation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

func TestDisplayUpdate(t *testing.T) {
	long := &config.GuiConfig{InstanceStatusStyle: "long"}
	newerTag := commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.31"}
	rebuilt := commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.27@sha256:0123456789ab"}

	tests := []struct {
		want                string
		changed, newerBuild bool
		verdict             commands.BeaconVerdict
	}{
		{want: ""},
		{want: "", verdict: commands.BeaconVerdict{Status: commands.BeaconCurrent}},
		{want: "↑1.31", verdict: newerTag},
		{want: "↑", verdict: rebuilt},
		{want: "↑", newerBuild: true, verdict: rebuilt},
		{want: "↻ ↑ ↑1.31", changed: true, newerBuild: true, verdict: newerTag},
		{want: "↻", changed: true},
		{want: "!", verdict: commands.BeaconVerdict{Status: commands.BeaconError, Error: "registry said 401"}},
		{want: "?", verdict: commands.BeaconVerdict{Status: commands.BeaconUnknown}},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, utils.Decolorise(displayUpdate(long, test.changed, test.newerBuild, test.verdict)), test)
	}

	alpine := commands.BeaconVerdict{
		Status: commands.BeaconUpdateAvailable, Latest: "1.31-alpine", Current: "docker.io/library/nginx:1.18-alpine",
	}
	assert.Equal(t, "↑1.31", utils.Decolorise(displayUpdate(long, false, false, alpine)))

	alpine.Current = "registry.example:5000/nginx:1.18"
	assert.Equal(t, "↑1.31-alpine", utils.Decolorise(displayUpdate(long, false, false, alpine)), "a variant only the new tag has stays")

	alpine.Current, alpine.Latest = "nginx:1.18-alpine3.20", "1.31-alpine3.21"
	assert.Equal(t, "↑1.31-alpine3.21", utils.Decolorise(displayUpdate(long, false, false, alpine)))

	short := &config.GuiConfig{InstanceStatusStyle: "short"}
	assert.Equal(t, "C ↑1.31", utils.Decolorise(displayUpdate(short, true, false, newerTag)))
}

func TestBeaconLine(t *testing.T) {
	since := time.Date(2026, 11, 10, 9, 12, 0, 0, time.Local)

	assert.Empty(t, BeaconLine(commands.BeaconVerdict{}))
	assert.Equal(t, "1.31 (incus-beacon since "+LocalTime(since)+")",
		BeaconLine(commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.31", Since: since}))
	assert.Equal(t, "none, the tag has a newer build (incus-beacon)",
		BeaconLine(commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.27@sha256:0123456789ab"}))
	assert.Equal(t, "none (incus-beacon)", BeaconLine(commands.BeaconVerdict{Status: commands.BeaconCurrent}))
	assert.Equal(t, "error: registry said 401 (incus-beacon)",
		BeaconLine(commands.BeaconVerdict{Status: commands.BeaconError, Error: "registry said 401"}))
	assert.Equal(t, "unknown (incus-beacon)", BeaconLine(commands.BeaconVerdict{Status: commands.BeaconUnknown}))
}

func TestSinceTime(t *testing.T) {
	now := time.Date(2026, 11, 11, 19, 40, 0, 0, time.Local)

	assert.Equal(t, now.Add(-time.Hour).Format("15:04 MST"), sinceTime(now.Add(-time.Hour), now))
	assert.Equal(t, LocalTime(now.AddDate(0, 0, -1)), sinceTime(now.AddDate(0, 0, -1), now))
}
