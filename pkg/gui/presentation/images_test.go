package presentation

import (
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func TestGetImageDisplayStrings(t *testing.T) {
	plain := func(cells []string) []string {
		return lo.Map(cells, func(cell string, _ int) string { return utils.Decolorise(cell) })
	}

	used := &commands.Image{
		Fingerprint: "0123456789abcdef",
		UsedBy:      []string{"default/web", "default/db"},
		Image: api.Image{
			Type: "virtual-machine", Size: 2 << 20, LastUsedAt: time.Date(2026, 9, 20, 12, 0, 0, 0, time.Local),
			Properties: map[string]string{"description": "Debian 13"},
		},
	}
	assert.Equal(t,
		[]string{"Debian 13", "2", "0123456789ab", "2.00MiB", "2026/09/20", "vm"},
		plain(GetImageDisplayStrings(used, false)))

	// Unaliased and undescribed, so labelled by fingerprint. The daemon's
	// "never" is the zero time, or the epoch from older ones.
	cached := &commands.Image{Fingerprint: "fedcba9876543210", Image: api.Image{
		Project: "default", Type: "container", Cached: true, LastUsedAt: time.Unix(0, 0),
	}}
	assert.Equal(t,
		[]string{"default", "fedcba987654", "0", "fedcba987654", "", "never", "cached"},
		plain(GetImageDisplayStrings(cached, true)))
}
