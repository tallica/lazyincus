package gui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

func testWarnings() []api.Warning {
	return []api.Warning{
		{
			UUID: "kvm", WarningPut: api.WarningPut{Status: "new"}, Type: "Instance type not operational",
			LastMessage: "KVM support is missing (no /dev/kvm)", Severity: "low", Count: 19,
			LastSeenAt: time.Now(),
		},
		{
			UUID: "dnsmasq", WarningPut: api.WarningPut{Status: "acknowledged"}, Type: "Skipping AppArmor for dnsmasq",
			Project: "default", Severity: "low", Count: 36, LastSeenAt: time.Now(),
		},
		{
			UUID: "disk", WarningPut: api.WarningPut{Status: "new"}, Type: "Storage pool almost full",
			Severity: "high", Count: 2, LastSeenAt: time.Now(),
		},
	}
}

func warningStatuses(s *screen) map[string]string {
	warnings, _ := s.server.GetWarnings()

	return lo.SliceToMap(warnings, func(warning api.Warning) (string, string) { return warning.UUID, warning.Status })
}

func TestScreenWarnings(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) { s.server.SetWarnings(testWarnings()) })
	s.ready(t)

	// The footer counts what no one has acknowledged.
	s.settle(t, "2 warnings")

	s.press(t, 'W')
	screen := s.settle(t, "Warnings (fake)")
	assert.Contains(t, screen, "KVM support is missing")

	// New first, the most severe first among them.
	s.do(t, func() error {
		assert.Equal(t, "disk", s.gui.State.Warnings[0].Key())
		return nil
	})

	s.press(t, 'a')
	assert.Eventually(t, func() bool { return warningStatuses(s)["disk"] == "acknowledged" }, 5*time.Second, 20*time.Millisecond)
	s.settle(t, "1 warning ")

	// The popup stays open on the same row, now acknowledged; d asks first.
	s.press(t, 'd')
	s.settle(t, `delete the warning "Storage pool almost`)
	s.pressKey(t, tcell.KeyEnter)

	assert.Eventually(t, func() bool { _, ok := warningStatuses(s)["disk"]; return !ok }, 5*time.Second, 20*time.Millisecond)
	s.settle(t, "Warnings (fake)")
}

func TestWarningKeysHonourReadOnly(t *testing.T) {
	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}, func(s *screen) { s.server.SetWarnings(testWarnings()) })
	s.ready(t)

	s.press(t, 'W')
	s.settle(t, "Warnings (fake)")
	s.press(t, 'a')
	s.settle(t, "fake is read-only")

	assert.Equal(t, "new", warningStatuses(s)["disk"])
}
