// Package tui is the Bubble Tea front end replacing pkg/gui; until it can,
// LAZYINCUS_TUI=bubbletea picks it.
package tui

import (
	"context"
	"os"
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
	// queued is what a fetch's apply started, which has no Cmd to return.
	queued []tea.Cmd
	tasks  *tasks.TaskManager
	// runTask stands in for pkg/tasks in tests.
	runTask func(task func(ctx context.Context, send func(tea.Msg))) tea.Cmd
	// mainParent is gocui's main ParentView: the panel enter came from.
	mainParent string
	filter     filter
	appStatus  string
	statuses   []appStatus
	spinning   bool

	composeAvailable bool
	// localStackDir and localStackExplicit are pkg/gui's: where the local stack comes from, set once.
	localStackDir      string
	localStackExplicit bool
	// loadStack reads a stack's compose config; tests stand in for incus-compose.
	loadStack func(dir string) *commands.ComposeStack
	// stackCache and remotes synchronise themselves: the fetches use them off the loop.
	stackCache *stackCache
	remotes    *remoteCommands
	// notifyRemotes tells the loop a remote connected or its statuses changed.
	notifyRemotes func()
	home          string
	stacks        []*commands.ComposeStack
	stacksHere    bool
	// stacksElsewhere is whether any listed stack is on another remote.
	stacksElsewhere bool
	// landing is pkg/gui's State.Landing: the first stacks read of a scope leaves Stacks if none is here.
	landing bool
	// stackDirs and stackRemotes are pkg/gui's, for the tabs naming a compose instance's stack.
	stackDirs     map[string]string
	stackRemotes  []string
	stackServices map[string]map[string]bool
	// selectedStack is what Services and Backups follow; nil for none.
	selectedStack      *commands.ComposeStack
	composeState       *composeHolder
	serviceRows        []*commands.ServiceRow
	servicesNote       string
	showStackInstances bool
	snapshotsShowAll   bool
	snapshotsFollow    snapshotsFollow
	snapshotsSpan      snapshotsSpan
	snapshots          []*commands.Snapshot

	instances     []*commands.Instance
	images        []*commands.Image
	imagesSpan    bool
	volumes       []*commands.Volume
	volumesSpan   bool
	networks      []*commands.Network
	networksSpan  bool
	showUnmanaged bool
	profiles      []*commands.Profile
	profilesSpan  bool
	// networkTabs is pkg/gui's: the forward and ACL events, which the networks' tabs show.
	networkTabs uint64
	// upstreams are the registry checks by image fingerprint, nil before the first.
	upstreams     map[string]commands.UpstreamStatus
	showProject   bool
	showStopped   bool
	instanceUsers *instanceUsers

	refreshes refreshSeqs
	events    events
	// inbox is what goroutines outside the program send it.
	inbox        chan tea.Msg
	streamEvents bool
	// eventsLive is whether an event stream is open: the slow polls wait on it.
	eventsLive bool
	// polledAt is when each slow poll last asked the daemon.
	polledAt       map[refreshKind]time.Time
	connectionLost bool
	configModTime  time.Time
	// checkUpstreams asks the registries; tests stand in for them.
	checkUpstreams func(context.Context, []commands.UpstreamTarget) map[string]commands.UpstreamStatus
	// runIncus is pkg/gui's, run off the loop; tests stand in for the CLI.
	runIncus func(*exec.Cmd) (string, error)
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
		showStopped:      true,
		filter:           filter{input: newEditorInput()},
		inbox:            make(chan tea.Msg, inboxSize),
		tasks:            tasks.NewTaskManager(incus.Log, tr),
		streamEvents:     true,
		runIncus:         osCommand.RunExecutableWithOutput,
		checkUpstreams:   commands.NewRegistryChecker().CheckAll,
	}
	if _, err := exec.LookPath("incus-compose"); err == nil {
		m.composeAvailable = true
	}
	m.home, _ = os.UserHomeDir()
	m.localStackDir, m.localStackExplicit = localStackDir()
	m.loadStack = incus.LoadComposeStack
	m.stackCache = &stackCache{}
	m.composeState = &composeHolder{}
	m.stacksHere, m.landing = true, true
	m.notifyRemotes = func() {
		select {
		case m.inbox <- remotesChangedMsg{}:
		default: // a full inbox: the next poll asks again
		}
	}
	m.remotes = &remoteCommands{
		connect: incus.ConnectRemote, known: incus.IsInstanceRemote,
		names: incus.InstanceRemoteNames, connected: m.notifyRemotes,
	}
	m.panels = m.newPanels()

	return m
}

func Run(incus *commands.IncusCommand, osCommand *commands.OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig) error {
	// fatih/color decides once, from stdout, whether to colour; Bubble Tea
	// owns the terminal, so the answer is always yes.
	color.NoColor = false

	if err := cfg.PinStacks(incus.RemoteName()); err != nil {
		incus.Log.Warn().Err(err).Send()
	}

	m := NewModel(incus, osCommand, tr, cfg)
	_, err := tea.NewProgram(m).Run()
	m.stopEvents()
	m.tasks.Close()

	return err
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.refreshAll(), m.pollers()}
	if m.cfg.UserConfig.Gui.CheckImageUpdatesOnStart {
		cmds = append(cmds, refreshQuiet(nil, m.imageUpdatesFetch(false)))
	}
	if m.streamEvents {
		cmds = append(cmds, m.listen(), m.receive())
	}

	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.fitViews()

	main, queued := m.pendingMain, m.queued
	m.pendingMain, m.queued = nil, nil

	return m, tea.Batch(append(queued, cmd, main)...)
}

// fitViews fits the lists and main to their windows: View only reads them.
func (m *Model) fitViews() {
	m.followSelection()
	m.fitStatus()
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

	case waitingDoneMsg:
		return m.onWaitingDone(msg)

	case heldStatusMsg:
		return m.onHeldStatus(msg)

	case transientDoneMsg:
		m.removeStatus(msg.name)

	case spinnerMsg:
		return m.onSpinner()

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

	case remotesChangedMsg:
		return refreshQuiet(nil, m.fetchStacks(), m.fetchServices())

	case pollMsg:
		return m.pollInstances()

	case slowPollMsg:
		return m.slowPoll(msg.kind)

	case syncConnectionMsg:
		m.syncConnection()

	case configPollMsg:
		return m.statConfig()

	case configStatMsg:
		return m.onConfigStat(msg.modTime)

	case execDoneMsg:
		return m.onExecDone(msg)

	case copiedMsg:
		return m.onCopied(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		switch {
		case m.currentView() == filterView:
			return m.editFilter(msg)
		case m.topPrompt() != nil && m.currentView() == confirmationView:
			return m.editPrompt(msg)
		}
	}

	return nil
}

func (m *Model) queue(cmd tea.Cmd) {
	m.queued = append(m.queued, cmd)
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
		return
	}

	if m.instanceUsers != nil && m.currentView() == "instances" {
		m.clearInstanceUsers()
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

// setMainTab is pkg/gui's SetMainTab: the tab named key, if p has one.
func (m *Model) setMainTab(p *panel, key string) {
	for i, tab := range p.mainTabs() {
		if tab.key == key {
			p.mainTab = i
		}
	}
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

	if p := m.topPrompt(); p != nil && m.currentView() == confirmationView {
		x0, y0, x1, _, _ := m.popupLayout(p)
		row, col := promptCursor(p.input, max(x1-x0-1, 1))
		view.Cursor = tea.NewCursor(x0+1+col, y0+1+row)
	}

	return view
}
