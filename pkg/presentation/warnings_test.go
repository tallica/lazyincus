package presentation

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func TestSortWarnings(t *testing.T) {
	now := time.Now()
	warning := func(uuid, status, severity string, seen time.Duration) *commands.Warning {
		return &commands.Warning{Warning: api.Warning{
			UUID: uuid, WarningPut: api.WarningPut{Status: status}, Severity: severity, LastSeenAt: now.Add(seen),
		}}
	}

	warnings := []*commands.Warning{
		warning("acked-high", "acknowledged", "high", 0),
		warning("new-low-old", "new", "low", -time.Hour),
		warning("new-low", "new", "low", 0),
		warning("new-high", "new", "high", -time.Hour),
	}
	SortWarnings(warnings)

	assert.Equal(t, []string{"new-high", "new-low", "new-low-old", "acked-high"},
		lo.Map(warnings, func(w *commands.Warning, _ int) string { return w.Warning.UUID }))
}
