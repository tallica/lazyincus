// Package tui is the Bubble Tea front end replacing pkg/gui; until it can,
// LAZYINCUS_TUI=bubbletea picks it.
package tui

import (
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

const pollInterval = 2 * time.Second

type pollMsg struct{}

// Model is the whole UI's state; Update is the only place it changes.
type Model struct {
	incus *commands.IncusCommand
	os    *commands.OSCommand
	tr    *i18n.TranslationSet
	cfg   *config.AppConfig
	theme theme

	width, height int

	panels []*panel
	// activeViews is each shared window's panel on show.
	activeViews map[string]string
	// focus is the static views, side panels and main, last on top.
	focus  []string
	popups []*popup

	screenMode      screenMode
	expandSidePanel bool
	main            pane
	// mainParent is gocui's main ParentView: the panel enter came from.
	mainParent string
	filter     filter
	appStatus  string

	composeAvailable bool
	// stacksHere stays false: nothing loads stacks yet.
	stacksHere       bool
	snapshotsShowAll bool

	instances   []*commands.Instance
	showProject bool

	// fetchSeq and appliedSeq stop a late fetch undoing a newer one.
	fetchSeq   int
	appliedSeq int
	loaded     bool
}

func NewModel(incus *commands.IncusCommand, osCommand *commands.OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig) *Model {
	gui := cfg.UserConfig.Gui

	m := &Model{
		incus:            incus,
		os:               osCommand,
		tr:               tr,
		cfg:              cfg,
		theme:            newTheme(gui),
		activeViews:      map[string]string{},
		screenMode:       parseScreenMode(gui.ScreenMode),
		expandSidePanel:  gui.ExpandFocusedSidePanel,
		snapshotsShowAll: gui.ShowAllSnapshots,
		filter:           filter{input: newFilterInput()},
	}
	if _, err := exec.LookPath("incus-compose"); err == nil {
		m.composeAvailable = true
	}
	m.panels = m.newPanels()
	m.landOffStacks()

	return m
}

func Run(incus *commands.IncusCommand, osCommand *commands.OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig) error {
	// fatih/color decides once, from stdout, whether to colour; Bubble Tea
	// owns the terminal, so the answer is always yes.
	color.NoColor = false

	_, err := tea.NewProgram(NewModel(incus, osCommand, tr, cfg)).Run()

	return err
}

func (m *Model) Init() tea.Cmd {
	return m.fetchInstances()
}

func poll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.fitViews()

	return m, cmd
}

// fitViews fits the lists and main to their windows: View only reads them.
func (m *Model) fitViews() {
	m.fitLists()
	m.fitMain()
	m.fitFilter()
	m.fitPopups()
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampLists()

	case instancesMsg:
		// Each answer re-arms the poll, so one is in flight at most.
		if msg.seq < m.appliedSeq {
			return nil
		}
		m.appliedSeq = msg.seq
		if msg.err == nil {
			m.setInstances(msg.instances)
			m.loaded = true
		}
		return poll()

	case pollMsg:
		return m.fetchInstances()

	case execDoneMsg:
		if msg.err != nil {
			m.openError(msg.err.Error())
		}
		return m.fetchInstances()

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		if m.currentView() == filterView {
			return m.editFilter(msg)
		}
	}

	return nil
}

func (m *Model) quit() tea.Cmd {
	if m.cfg.UserConfig.ConfirmOnQuit {
		m.openConfirmation("", m.tr.ConfirmQuit, func() tea.Cmd { return tea.Quit })
		return nil
	}

	return tea.Quit
}

// escape is pkg/gui's, for esc where no view takes it.
func (m *Model) escape() {
	if m.filter.active {
		m.clearFilter()
	}
}

func (m *Model) jumpToTopMain() {
	m.main.autoscroll = false
	m.main.ox, m.main.oy = 0, 0
}

// enterMain is pkg/gui's handleEnterMain.
func (m *Model) enterMain() {
	parent := m.currentView()
	m.focusView(mainView)
	m.mainParent = parent
}

func (m *Model) exitMain() {
	m.mainParent = ""
	m.returnFocus()
}

func (m *Model) moveSelection(p *panel, step int) {
	p.selectRow(p.selected+step, m.listRows(p))
}

func (m *Model) cycleMainTab(p *panel, step int) {
	if p.mainTabs == nil {
		return
	}

	p.mainTab = wrap(p.mainTab+step, len(p.mainTabs()))
}

// scrollMain is pkg/gui's scrollUpMain (-1) and scrollDownMain (1).
func (m *Model) scrollMain(direction int) {
	step := m.cfg.UserConfig.Gui.ScrollHeight
	if direction < 0 {
		m.main.scrollUp(step)
		return
	}

	_, h := size(m.dims()[mainView])
	m.main.scrollDown(step, h-2, m.cfg.UserConfig.Gui.ScrollPastBottom)
}

// fitMain refreshes the main pane from the tab on show.
func (m *Model) fitMain() {
	d, ok := m.dims()[mainView]
	if !ok {
		return
	}
	w, h := size(d)

	p := m.currentSidePanel()
	if p.count() == 0 {
		m.main.set(p.name, p.empty, m.cfg.UserConfig.Gui.WrapMainPanel, false, w-2, h-2)
		return
	}
	if p.mainTabs == nil {
		m.main.set(p.name, "", false, false, w-2, h-2)
		return
	}

	tab := p.mainTabs()[p.mainTab]
	key := p.name + "\x00" + tab.title
	if p.itemKey != nil {
		key += "\x00" + p.itemKey()
	}
	m.main.set(key, tab.content(), tab.wrap, tab.autoscroll, w-2, h-2)
}

func (m *Model) listRows(p *panel) int {
	d, ok := m.dims()[p.windowName()]
	if !ok {
		return 0
	}

	_, h := size(d)

	return h - 2
}

func (m *Model) fitLists() {
	if m.width == 0 {
		return
	}

	dims := m.dims()
	for _, p := range m.visiblePanels() {
		if d, ok := dims[p.windowName()]; ok {
			w, _ := size(d)
			p.fit(w - 2)
		}
	}
}

func (m *Model) clampLists() {
	if m.width == 0 {
		return
	}

	for _, p := range m.visiblePanels() {
		p.selectRow(p.selected, m.listRows(p))
	}
}

func (m *Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true

	if d, ok := m.dims()[filterView]; ok && m.currentView() == filterView {
		view.Cursor = tea.NewCursor(d.X0+m.filterCursorX()-m.filter.ox, d.Y0)
	}

	return view
}
