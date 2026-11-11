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

// Beacon answers Newer tag only when it looks for tags; otherwise, and
// for a rebuild whatever it looks for, its answer is about the build.
func TestBeaconLines(t *testing.T) {
	since := time.Date(2026, 11, 10, 9, 12, 0, 0, time.Local)
	lines := func(verdict commands.BeaconVerdict) []string {
		build, tag := BeaconLines(verdict)
		return []string{build, tag}
	}

	assert.Equal(t, []string{"", ""}, lines(commands.BeaconVerdict{}))
	assert.Equal(t, []string{"", "1.31 (incus-beacon since " + LocalTime(since) + ")"},
		lines(commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.31", Since: since, Semver: true}))
	assert.Equal(t, []string{"", "none (incus-beacon)"}, lines(commands.BeaconVerdict{Status: commands.BeaconCurrent, Semver: true}))

	rebuilt := commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.27@sha256:0123456789ab"}
	assert.Equal(t, []string{"yes, G takes it (incus-beacon)", ""}, lines(rebuilt))
	rebuilt.Semver = true
	assert.Equal(t, []string{"yes, G takes it (incus-beacon)", ""}, lines(rebuilt))

	assert.Equal(t, []string{"none (incus-beacon)", ""}, lines(commands.BeaconVerdict{Status: commands.BeaconCurrent}))
	assert.Equal(t, []string{"", "1.31 (incus-beacon)"}, lines(commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.31"}),
		"a newer tag is one whichever key asked for it")
	assert.Equal(t, []string{"error: registry said 401 (incus-beacon)", ""},
		lines(commands.BeaconVerdict{Status: commands.BeaconError, Error: "registry said 401"}))
	assert.Equal(t, []string{"", "unknown (incus-beacon)"}, lines(commands.BeaconVerdict{Status: commands.BeaconUnknown, Semver: true}))
}

func TestSinceTime(t *testing.T) {
	now := time.Date(2026, 11, 11, 19, 40, 0, 0, time.Local)

	assert.Equal(t, now.Add(-time.Hour).Format("15:04 MST"), sinceTime(now.Add(-time.Hour), now))
	assert.Equal(t, LocalTime(now.AddDate(0, 0, -1)), sinceTime(now.AddDate(0, 0, -1), now))
}

// Either source finding a newer build wins the line; else g's answer does.
func TestNewerBuildLine(t *testing.T) {
	rebuilt := commands.BeaconVerdict{Status: commands.BeaconUpdateAvailable, Latest: "1.27@sha256:0123456789ab"}
	current := commands.BeaconVerdict{Status: commands.BeaconCurrent}
	newer := commands.UpstreamStatus{State: commands.UpstreamNewer}
	none := commands.UpstreamStatus{State: commands.UpstreamCurrent}

	line := func(upstream commands.UpstreamStatus, verdict commands.BeaconVerdict) string {
		build, _ := BeaconLines(verdict)
		return newerBuildLine(upstream, verdict, build)
	}

	assert.Equal(t, "yes, G takes it", line(newer, current))
	assert.Equal(t, "yes, G takes it (incus-beacon)", line(none, rebuilt))
	assert.Equal(t, "none", line(none, current))
	assert.Equal(t, "none (incus-beacon)", line(commands.UpstreamStatus{}, current))
	assert.Empty(t, line(commands.UpstreamStatus{}, commands.BeaconVerdict{}))
}
