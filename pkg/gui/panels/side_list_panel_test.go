package panels

import (
	"context"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// stubGui is the smallest IGui that FilterAndSort needs: no filtering, no
// ignore strings, nothing rendered.
type stubGui struct{}

func (stubGui) HandleClick(*gocui.View, int, *int, func() error) error { return nil }
func (stubGui) NewSimpleRenderStringTask(func() string) tasks.TaskFunc { return nil }
func (stubGui) FocusY(int, int, *gocui.View)                           {}
func (stubGui) ShouldRefresh(string) bool                              { return true }
func (stubGui) GetMainView() *gocui.View                               { return nil }
func (stubGui) IsCurrentView(*gocui.View) bool                         { return false }
func (stubGui) FilterString(*gocui.View) string                        { return "" }
func (stubGui) IgnoreStrings() []string                                { return nil }
func (stubGui) Update(func() error)                                    {}
func (stubGui) QueueTask(func(ctx context.Context)) error              { return nil }

type row struct {
	name    string
	stopped bool
}

func newPanel(items []*row) *SideListPanel[*row] {
	panel := &SideListPanel[*row]{
		ListPanel: ListPanel[*row]{List: NewFilteredList[*row]()},
		Gui:       stubGui{},
		Sort: func(a, b *row) bool {
			if a.stopped != b.stopped {
				return b.stopped
			}
			return a.name < b.name
		},
		GetTableCells: func(item *row) []string { return []string{item.name} },
	}

	panel.SetItems(items)

	return panel
}

func selectedName(t *testing.T, panel *SideListPanel[*row]) string {
	t.Helper()

	item, err := panel.GetSelectedItem()
	assert.NoError(t, err)

	return item.name
}

func TestSelectionFollowsItemAcrossResort(t *testing.T) {
	alpha := &row{name: "alpha"}
	bravo := &row{name: "bravo"}
	charlie := &row{name: "charlie"}

	panel := newPanel([]*row{alpha, bravo, charlie})
	panel.SetSelectedLineIdx(0)
	assert.Equal(t, "alpha", selectedName(t, panel))

	// alpha stops and sorts to the bottom; the cursor should go with it
	// rather than stay on row 0, which bravo now occupies.
	alpha.stopped = true
	panel.RerenderList()

	assert.Equal(t, "alpha", selectedName(t, panel))
	assert.Equal(t, 2, panel.SelectedIdx)
}

// The refresh path the background poll actually takes: SetItems with a fresh
// slice, in whatever order the daemon returned it.
func TestSelectionFollowsItemAcrossSetItems(t *testing.T) {
	alpha := &row{name: "alpha"}
	bravo := &row{name: "bravo"}
	charlie := &row{name: "charlie"}

	panel := newPanel([]*row{alpha, bravo, charlie})
	panel.SetSelectedLineIdx(0)
	assert.Equal(t, "alpha", selectedName(t, panel))

	alpha.stopped = true
	panel.SetItems([]*row{charlie, alpha, bravo})

	assert.Equal(t, "alpha", selectedName(t, panel))
	assert.Equal(t, 2, panel.SelectedIdx)
}

func TestSelectionClampsWhenSelectedItemDisappears(t *testing.T) {
	alpha := &row{name: "alpha"}
	bravo := &row{name: "bravo"}

	panel := newPanel([]*row{alpha, bravo})
	panel.SetSelectedLineIdx(1)
	assert.Equal(t, "bravo", selectedName(t, panel))

	panel.SetItems([]*row{alpha})

	assert.Equal(t, "alpha", selectedName(t, panel))
	assert.Equal(t, 0, panel.SelectedIdx)
}
