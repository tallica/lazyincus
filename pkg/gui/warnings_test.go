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

// openWarnings opens W's popup, which opens on the warnings while any are
// new.
func openWarnings(t *testing.T, s *screen) string {
	t.Helper()

	s.settle(t, " warning")
	s.press(t, 'W')

	return s.settle(t, "a: acknowledge, d: delete")
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

	screen := openWarnings(t, s)
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
	// Saying no goes back to the list.
	s.press(t, 'd')
	s.settle(t, "delete this warning")
	s.press(t, 'n')
	s.settle(t, "a: acknowledge, d: delete")

	s.press(t, 'd')
	screen = s.settle(t, "delete this warning")
	assert.Contains(t, screen, "Storage pool almost full")
	s.pressKey(t, tcell.KeyEnter)

	assert.Eventually(t, func() bool { _, ok := warningStatuses(s)["disk"]; return !ok }, 5*time.Second, 20*time.Millisecond)
	s.settle(t, "a: acknowledge, d: delete")
}

// Closing a warning's details goes back to the list, on that warning.
func TestClosingAWarningsDetails(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) { s.server.SetWarnings(testWarnings()) })
	s.ready(t)

	openWarnings(t, s)
	s.pressKey(t, tcell.KeyDown)
	s.pressKey(t, tcell.KeyEnter)
	s.settle(t, "First seen:")
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "a: acknowledge, d: delete")

	s.do(t, func() error {
		item, err := s.gui.Panels.Menu.GetSelectedItem()
		assert.NoError(t, err)
		assert.Contains(t, item.FilterText, "KVM support is missing")
		return nil
	})
}

func TestWarningKeysHonourReadOnly(t *testing.T) {
	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}, func(s *screen) { s.server.SetWarnings(testWarnings()) })
	s.ready(t)

	openWarnings(t, s)
	s.press(t, 'a')
	s.settle(t, "fake is read-only")

	assert.Equal(t, "new", warningStatuses(s)["disk"])
}

// W opens on the warnings while any are new, then on the list last shown.
func TestWOpensOnNewWarningsThenTheLastList(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, func(s *screen) { s.server.SetWarnings(testWarnings()) })
	s.ready(t)

	openWarnings(t, s)
	s.press(t, '[')
	s.settle(t, "d: cancel, enter: details")
	s.pressKey(t, tcell.KeyEsc)
	s.settle(t, "x: menu")

	s.press(t, 'W')
	s.settle(t, "d: cancel, enter: details")
}
