// Package tui is the Bubble Tea front end replacing pkg/gui; until it can,
// LAZYINCUS_TUI=bubbletea picks it.
package tui

import (
	"context"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/tasks"
)

const inboxSize = 256

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
	mainTask        mainTask
	// pendingMain starts the task fitMain queued, for Update to return.
	pendingMain tea.Cmd
	tasks       *tasks.TaskManager
	// runTask stands in for pkg/tasks in tests.
	runTask func(task func(ctx context.Context, send func(tea.Msg))) tea.Cmd
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

	refreshes refreshSeqs
	events    events
	// inbox is what goroutines outside the program send it.
	inbox          chan tea.Msg
	streamEvents   bool
	connectionLost bool
	configModTime  time.Time
	// after stands in for tea.Tick in tests.
	after func(time.Duration, tea.Msg) tea.Cmd
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
		filter:           filter{input: newEditorInput()},
		inbox:            make(chan tea.Msg, inboxSize),
		tasks:            tasks.NewTaskManager(incus.Log, tr),
		streamEvents:     true,
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

	m := NewModel(incus, osCommand, tr, cfg)
	_, err := tea.NewProgram(m).Run()
	m.stopEvents()
	m.tasks.Close()

	return err
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.refreshAll(), m.pollers()}
	if m.streamEvents {
		cmds = append(cmds, m.listen(), m.receive())
	}

	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.fitViews()

	main := m.pendingMain
	m.pendingMain = nil

	return m, tea.Batch(cmd, main)
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

	case refreshMsg:
		return m.applyRefresh(msg)

	case mainWriteMsg:
		m.onMainWrite(msg)

	case received:
		return tea.Batch(m.update(msg.msg), m.receive())

	case eventsOpenedMsg:
		return m.onEventsOpened(msg)

	case eventMsg:
		return m.onEvent(msg)

	case eventsClosedMsg:
		return m.onEventsClosed(msg)

	case reopenEventsMsg:
		if msg.stream == m.events.stream {
			return m.listen()
		}

	case flushEventsMsg:
		return m.flushEvents()

	case instanceUpdatedMsg:
		return m.refreshUpdated()

	case pollMsg:
		return m.pollInstances()

	case syncConnectionMsg:
		m.syncConnection()

	case configPollMsg:
		return m.statConfig()

	case configStatMsg:
		return m.onConfigStat(msg.modTime)

	case execDoneMsg:
		if msg.err != nil {
			m.openError(msg.err.Error())
		}
		return refresh(m.fetchInstances())

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		switch {
		case m.currentView() == filterView:
			return m.editFilter(msg)
		case m.topPrompt() != nil:
			return m.editPrompt(msg)
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

// fitMain sizes main and queues the task for what it should show.
func (m *Model) fitMain() {
	d, ok := m.dims()[mainView]
	if !ok {
		return
	}
	w, h := size(d)
	m.main.fit(w-2, h-2)

	p := m.currentSidePanel()
	if p.count() == 0 {
		message := p.empty
		if p.awaiting != "" {
			message = p.awaiting
		}
		m.queueMain(p.name+"\x00"+message, m.cfg.UserConfig.Gui.WrapMainPanel, false, renderString(func() string { return message }))
		return
	}
	if p.mainTabs == nil {
		m.queueMain(p.name, false, false, renderString(func() string { return "" }))
		return
	}

	tab := p.mainTabs()[p.mainTab]
	key := p.name + "\x00" + tab.title
	if p.itemKey != nil {
		key += "\x00" + p.itemKey()
	}
	m.queueMain(key, tab.wrap, tab.autoscroll, tab.task())
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

	if p := m.topPrompt(); p != nil {
		x0, y0, x1, _, _ := m.popupLayout(p)
		row, col := promptCursor(p.input, max(x1-x0-1, 1))
		view.Cursor = tea.NewCursor(x0+1+col, y0+1+row)
	}

	return view
}
