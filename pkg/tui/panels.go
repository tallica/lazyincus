package tui

import (
	"slices"
	"strings"

	"github.com/tallica/lazyincus/pkg/utils"
)

// panel is pkg/gui's sidePanelDef as data, plus its list's state.
type panel struct {
	name string
	// window, plainTitle and shortTitle are as on sidePanelDef.
	window     string
	plainTitle string
	shortTitle string
	title      func() string
	hidden     func() bool

	// cells are every item's columns, shown those on show; flex narrow.
	cells [][]string
	shown []int
	flex  func() []utils.FlexColumn
	// empty is pkg/gui's NoItemsMessage: main shows it, the list nothing.
	empty    string
	mainTabs func() []mainTab
	// itemKey is pkg/gui's GetItemContextCacheKey for the selected row.
	itemKey func() string

	// rows are cells rendered at rowsWidth, -1 once cells change.
	rows      []string
	rowsWidth int

	selected int
	origin   int
	mainTab  int
}

type mainTab struct {
	title      string
	content    func() string
	wrap       bool
	autoscroll bool
}

func (p *panel) windowName() string {
	if p.window == "" {
		return p.name
	}

	return p.window
}

func (p *panel) isHidden() bool {
	return p.hidden != nil && p.hidden()
}

func (p *panel) count() int {
	return len(p.shown)
}

// item is the selected row's index into cells, -1 for none.
func (p *panel) item() int {
	if p.selected < 0 || p.selected >= len(p.shown) {
		return -1
	}

	return p.shown[p.selected]
}

// filter is pkg/gui's filterAndSort for a sorted list: matched as typed,
// colour codes and all; the selection follows its item.
func (p *panel) filter(needle string, ignore []string) {
	item := p.item()

	contains := func(cells []string, s string) bool {
		return slices.ContainsFunc(cells, func(cell string) bool { return strings.Contains(cell, s) })
	}

	p.shown = p.shown[:0]
	for i, cells := range p.cells {
		if slices.ContainsFunc(ignore, func(s string) bool { return contains(cells, s) }) {
			continue
		}
		if needle != "" && !contains(cells, needle) {
			continue
		}
		p.shown = append(p.shown, i)
	}
	p.rowsWidth = -1

	if i := slices.Index(p.shown, item); item >= 0 && i >= 0 {
		p.selected = i
	}
	p.selected = max(min(p.selected, len(p.shown)-1), 0)
}

// fit renders the rows to width as pkg/gui's writeRows does, each cut row
// marked with an ellipsis; a no-op at the width they're already at.
func (p *panel) fit(width int) {
	if width == p.rowsWidth {
		return
	}
	p.rowsWidth = width

	var flex []utils.FlexColumn
	if p.flex != nil {
		flex = p.flex()
	}

	cells := make([][]string, len(p.shown))
	for i, item := range p.shown {
		cells[i] = p.cells[item]
	}

	table, err := utils.RenderTableToWidth(cells, width, flex)
	if err != nil {
		p.rows = []string{err.Error()}
		return
	}

	p.rows = nil
	if len(cells) == 0 {
		return
	}
	for _, row := range strings.Split(table, "\n") {
		p.rows = append(p.rows, utils.Truncate(row, width))
	}
}

// selectRow clamps the selection and scrolls just enough to show it.
func (p *panel) selectRow(row, rows int) {
	p.selected = max(min(row, p.count()-1), 0)
	if p.selected < p.origin {
		p.origin = p.selected
	}
	if rows > 0 && p.selected >= p.origin+rows {
		p.origin = p.selected - rows + 1
	}
	p.origin = max(p.origin, 0)
}

const (
	snapshotsWindow = "restorePoints"
	resourcesWindow = "resources"
)

// newPanels is pkg/gui's sidePanelDefs, in its order.
func (m *Model) newPanels() []*panel {
	tr := m.tr
	static := func(s string) func() string { return func() string { return s } }

	return []*panel{
		{name: "stacks", title: static(tr.StacksTitle), hidden: m.composeUnavailable, empty: tr.NoStacks},
		{name: "services", title: static(tr.ServicesTitle), hidden: m.composeUnavailable, empty: tr.NoServices},
		{
			name: "instances", title: static(tr.InstancesTitle), empty: tr.NoInstances,
			flex: m.instanceFlex, mainTabs: m.instanceMainTabs, itemKey: m.instanceKey,
		},
		{
			name: "snapshots", title: m.snapshotsTitle, window: snapshotsWindow,
			plainTitle: tr.SnapshotsTitle, shortTitle: tr.SnapshotsShort, empty: tr.NoSnapshots,
		},
		{
			name: "backups", title: static(tr.BackupsTitle), hidden: m.backupsAway, window: snapshotsWindow,
			plainTitle: tr.BackupsTitle, shortTitle: tr.BackupsShort, empty: tr.NoBackups,
		},
		{name: "images", title: static(tr.ImagesTitle), window: resourcesWindow, shortTitle: tr.ImagesShort, empty: tr.NoImages},
		{name: "volumes", title: static(tr.VolumesTitle), window: resourcesWindow, shortTitle: tr.VolumesShort, empty: tr.NoVolumes},
		{name: "networks", title: static(tr.NetworksTitle), window: resourcesWindow, shortTitle: tr.NetworksShort, empty: tr.NoNetworks},
		{name: "profiles", title: static(tr.ProfilesTitle), window: resourcesWindow, shortTitle: tr.ProfilesShort, empty: tr.NoProfiles},
	}
}

func (m *Model) composeUnavailable() bool {
	return !m.composeAvailable
}

func (m *Model) backupsAway() bool {
	return m.composeUnavailable() || m.stacksAway()
}

// stacksAway is pkg/gui's stacksAway.
func (m *Model) stacksAway() bool {
	return !m.stacksHere && m.cfg.UserConfig.Gui.CollapseStacksElsewhere
}

// snapshotsTitle is pkg/gui's snapshotsTabTitle.
func (m *Model) snapshotsTitle() string {
	if m.snapshotsShowAll {
		return m.tr.SnapshotsTitle + " (" + m.tr.AllSnapshotsLabel + ")"
	}

	return m.tr.SnapshotsTitle
}
