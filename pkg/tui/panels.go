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
	// keep is pkg/gui's Filter: an item it refuses is never shown.
	keep func(item int) bool
	// fuzzy is pkg/gui's FuzzyFilter, matching filterText or the cells.
	fuzzy      bool
	filterText func(item int) string
	// empty is pkg/gui's NoItemsMessage: main shows it, the list nothing.
	empty string
	// awaiting stands in for the list, and in main, until its first answer.
	awaiting string
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
	key   string
	title string
	// task makes, on the loop, the task that writes the tab off it.
	task       func() mainTaskFunc
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
// colour codes and all, the selection following its item; or fuzzily,
// best first, the selection on the first.
func (p *panel) filter(needle string, ignore []string) {
	item := p.item()
	fuzzy := p.fuzzy && needle != ""

	contains := func(cells []string, s string) bool {
		return slices.ContainsFunc(cells, func(cell string) bool { return strings.Contains(cell, s) })
	}

	p.shown = p.shown[:0]
	for i, cells := range p.cells {
		if p.keep != nil && !p.keep(i) {
			continue
		}
		if slices.ContainsFunc(ignore, func(s string) bool { return contains(cells, s) }) {
			continue
		}
		if needle != "" && !fuzzy && !contains(cells, needle) {
			continue
		}
		p.shown = append(p.shown, i)
	}
	p.rowsWidth = -1

	if fuzzy {
		p.fuzzyFilter(needle)
		return
	}

	if i := slices.Index(p.shown, item); item >= 0 && i >= 0 {
		p.selected = i
	}
	p.selected = max(min(p.selected, len(p.shown)-1), 0)
}

func (p *panel) fuzzyFilter(needle string) {
	texts := make([]string, len(p.shown))
	for i, item := range p.shown {
		if p.filterText != nil {
			texts[i] = p.filterText(item)
		}
		if texts[i] == "" {
			texts[i] = utils.Decolorise(strings.Join(p.cells[item], " "))
		}
	}

	ranked := utils.FuzzyRank(texts, needle)
	shown := make([]int, len(ranked))
	for i, index := range ranked {
		shown[i] = p.shown[index]
	}
	p.shown = shown
	p.selected = 0
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
		if p.awaiting != "" {
			p.rows = []string{utils.Truncate(p.awaiting, width)}
		}
		return
	}
	for _, row := range strings.Split(table, "\n") {
		p.rows = append(p.rows, utils.Truncate(row, width))
	}
}

// refill is pkg/gui's SetItems and RerenderList: replace swaps the items
// and returns their cells, the selection kept on the item key named.
func (m *Model) refill(p *panel, key func(item int) string, replace func() [][]string) {
	selected := ""
	if item := p.item(); item >= 0 && item < len(p.cells) {
		selected = key(item)
	}

	p.awaiting = ""
	p.cells = replace()
	m.filterList(p)
	if row := slices.IndexFunc(p.shown, func(item int) bool { return key(item) == selected }); selected != "" && row >= 0 {
		p.selected = row
	}
	p.selectRow(p.selected, m.listRows(p))
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

	panels := []*panel{
		{name: "stacks", title: static(tr.StacksTitle), hidden: m.composeUnavailable, empty: tr.NoStacks},
		{name: "services", title: static(tr.ServicesTitle), hidden: m.composeUnavailable, empty: tr.NoServices},
		{
			name: "instances", title: m.instancesTitle, empty: tr.NoInstances,
			flex: m.instanceFlex, mainTabs: m.instanceMainTabs, itemKey: m.instanceKey, keep: m.keepInstance,
		},
		{
			name: "snapshots", title: m.snapshotsTitle, window: snapshotsWindow,
			plainTitle: tr.SnapshotsTitle, shortTitle: tr.SnapshotsShort, empty: tr.NoSnapshots,
			flex: m.snapshotFlex, mainTabs: m.snapshotMainTabs, itemKey: m.snapshotKey,
		},
		{
			name: "backups", title: static(tr.BackupsTitle), hidden: m.backupsAway, window: snapshotsWindow,
			plainTitle: tr.BackupsTitle, shortTitle: tr.BackupsShort, empty: tr.NoBackups,
		},
		{
			name: "images", title: static(tr.ImagesTitle), window: resourcesWindow, shortTitle: tr.ImagesShort, empty: tr.NoImages,
			flex: m.imageFlex, mainTabs: m.imageMainTabs, itemKey: m.imageKey,
		},
		{
			name: "volumes", title: static(tr.VolumesTitle), window: resourcesWindow, shortTitle: tr.VolumesShort, empty: tr.NoVolumes,
			flex: m.volumeFlex, mainTabs: m.volumeMainTabs, itemKey: m.volumeKey,
		},
		{
			name: "networks", title: static(tr.NetworksTitle), window: resourcesWindow, shortTitle: tr.NetworksShort, empty: tr.NoNetworks,
			mainTabs: m.networkMainTabs, itemKey: m.networkKey, keep: m.keepNetwork,
		},
		{
			name: "profiles", title: static(tr.ProfilesTitle), window: resourcesWindow, shortTitle: tr.ProfilesShort, empty: tr.NoProfiles,
			flex: m.profileFlex, mainTabs: m.profileMainTabs, itemKey: m.profileKey,
		},
	}
	for _, p := range panels {
		p.awaiting = tr.Loading
	}

	return panels
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
