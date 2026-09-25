package panels

import (
	"context"
	"errors"
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
	assert.NoError(t, panel.RerenderList())

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

// A panel whose items a refresh rebuilds - the services panel's rows - says
// how to recognize the same row across that, and the cursor follows it the
// way it does an item that kept its pointer.
func TestSelectionFollowsRebuiltItem(t *testing.T) {
	panel := newPanel([]*row{{name: "alpha"}, {name: "bravo"}})
	panel.SameItem = func(a, b *row) bool { return a.name == b.name }
	panel.SetSelectedLineIdx(1)

	panel.SetItems([]*row{{name: "alpha"}, {name: "avocado"}, {name: "bravo"}})

	assert.Equal(t, "bravo", selectedName(t, panel))
	assert.Equal(t, 2, panel.SelectedIdx)
}

// renderingGui runs Update's function in place, so RerenderList writes to
// the view before returning.
type renderingGui struct{ stubGui }

func (renderingGui) Update(f func() error) { _ = f() }

func TestRowsRefitWhenPanelResizes(t *testing.T) {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 40, Height: 10})
	assert.NoError(t, err)
	defer g.Close()

	view, err := g.SetView("list", 0, 0, 11, 5, 0)
	if !errors.Is(err, gocui.ErrUnknownView) {
		assert.NoError(t, err)
	}

	panel := newPanel([]*row{{name: "alpha-beta-gamma"}, {name: "delta"}})
	panel.Gui = renderingGui{}
	panel.View = view

	assert.NoError(t, panel.RerenderList())
	assert.Equal(t, []string{"alpha-bet…", "delta"}, view.BufferLines())

	_, err = g.SetView("list", 0, 0, 21, 5, 0)
	assert.NoError(t, err)
	panel.FitToWidth()

	assert.Equal(t, []string{"alpha-beta-gamma", "delta"}, view.BufferLines())
}
