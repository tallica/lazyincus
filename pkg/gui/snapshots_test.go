package gui

import (
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// n on Snapshots snapshots what the panel follows, not whatever Instances
// has selected: here its first row, while the panel follows others.
func TestNewSnapshotFollowsThePanel(t *testing.T) {
	follow := func(t *testing.T, names ...string) *screen {
		t.Helper()

		s := startScreen(t, 140, 40, nil)
		s.ready(t)
		s.do(t, func() error {
			s.gui.State.SnapshotsInstances = lo.Filter(s.gui.Panels.Instances.List.GetAllItems(), func(instance *commands.Instance, _ int) bool {
				return slices.Contains(names, instance.Name)
			})

			return s.gui.handleSnapshotCreate(s.g, s.gui.Views.Snapshots)
		})

		return s
	}

	t.Run("instance", func(t *testing.T) {
		follow(t, "web").settle(t, "New snapshot of web")
	})

	t.Run("replicas", func(t *testing.T) {
		s := follow(t, "web", "db")
		s.settle(t, "─new snapshot─")
		labels := onLoop(t, s, func() []string {
			return lo.Map(s.gui.Panels.Menu.List.GetAllItems(), func(item *types.MenuItem, _ int) string { return item.LabelColumns[0] })
		})
		assert.ElementsMatch(t, []string{"web", "db", s.gui.Tr.Cancel}, labels)
	})
}
