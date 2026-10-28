package tui

// panel is pkg/gui's sidePanelDef as data, plus its list's state.
type panel struct {
	name string
	// window, plainTitle and shortTitle are as on sidePanelDef.
	window     string
	plainTitle string
	shortTitle string
	title      func() string
	hidden     func() bool

	// lines are the rows, rendered; empty is what an empty list says.
	lines    func() []string
	empty    string
	mainTabs func() []mainTab

	selected int
	origin   int
	mainTab  int
}

type mainTab struct {
	title   string
	content func() string
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
	if p.lines == nil {
		return 0
	}

	return len(p.lines())
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
			lines: m.instanceLines, mainTabs: m.instanceMainTabs,
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
