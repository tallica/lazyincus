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
	mainOrigin      int
	appStatus       string

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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampLists()

	case instancesMsg:
		// Each answer re-arms the poll, so one is in flight at most.
		if msg.seq < m.appliedSeq {
			return m, nil
		}
		m.appliedSeq = msg.seq
		if msg.err == nil {
			m.setInstances(msg.instances)
			m.loaded = true
		}
		return m, poll()

	case pollMsg:
		return m, m.fetchInstances()

	case execDoneMsg:
		if msg.err != nil {
			m.openError(msg.err.Error())
		}
		return m, m.fetchInstances()

	case tea.KeyPressMsg:
		return m, m.handleKey(msg.String())
	}

	return m, nil
}

// handleKey tries the top popup, then the focused view, then global keys.
func (m *Model) handleKey(key string) tea.Cmd {
	if len(m.popups) > 0 {
		if cmd, handled := m.popupKey(key); handled {
			return cmd
		}
	} else if handled, cmd := m.viewKey(key); handled {
		return cmd
	}

	return m.globalKey(key)
}

func (m *Model) viewKey(key string) (bool, tea.Cmd) {
	view := m.currentView()

	if view == mainView {
		switch key {
		case "esc":
			m.returnFocus()
		case "k", "up":
			m.scrollMain(-m.cfg.UserConfig.Gui.ScrollHeight)
		case "j", "down":
			m.scrollMain(m.cfg.UserConfig.Gui.ScrollHeight)
		case "h", "left", "l", "right":
			// Sideways scrolling comes with the scrollable pane.
		default:
			return false, nil
		}
		return true, nil
	}

	p := m.panelNamed(view)
	if p == nil {
		return false, nil
	}

	switch key {
	case "k", "up":
		m.moveSelection(p, -1)
	case "j", "down":
		m.moveSelection(p, 1)
	case "enter":
		m.focusView(mainView)
	case "[":
		m.cycleMainTab(p, -1)
	case "]":
		m.cycleMainTab(p, 1)
	case "E":
		if p.name != "instances" {
			return false, nil
		}
		return true, m.execShell()
	default:
		return false, nil
	}

	return true, nil
}

func (m *Model) globalKey(key string) tea.Cmd {
	switch key {
	case "q", "ctrl+c":
		if m.cfg.UserConfig.ConfirmOnQuit {
			m.openConfirmation("", m.tr.ConfirmQuit, func() tea.Cmd { return tea.Quit })
			return nil
		}
		return tea.Quit
	case "pgup", "ctrl+u":
		m.scrollMain(-m.cfg.UserConfig.Gui.ScrollHeight)
	case "pgdown", "ctrl+d":
		m.scrollMain(m.cfg.UserConfig.Gui.ScrollHeight)
	case "home":
		m.mainOrigin = 0
	case "tab":
		m.cycleWindows(1)
	case "shift+tab":
		m.cycleWindows(-1)
	case "h", "left":
		m.cycleViews(-1)
	case "l", "right":
		m.cycleViews(1)
	case "+":
		m.nextScreenMode(1)
	case "_":
		m.nextScreenMode(-1)
	case "=":
		m.expandSidePanel = !m.expandSidePanel
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if windows := m.windowNames(); int(key[0]-'1') < len(windows) {
				m.goToWindow(windows[key[0]-'1'])
				m.mainOrigin = 0
			}
		}
	}

	m.clampLists()

	return nil
}

func (m *Model) moveSelection(p *panel, step int) {
	before := p.selected
	p.selectRow(p.selected+step, m.listRows(p))
	if p.selected != before {
		m.mainOrigin = 0
	}
}

func (m *Model) cycleMainTab(p *panel, step int) {
	if p.mainTabs == nil {
		return
	}

	p.mainTab = wrap(p.mainTab+step, len(p.mainTabs()))
	m.mainOrigin = 0
}

// scrollMain is pkg/gui's scrollUpMain and scrollDownMain.
func (m *Model) scrollMain(lines int) {
	if lines < 0 {
		m.mainOrigin = max(m.mainOrigin+lines, 0)
		return
	}

	reserved := 0
	if !m.cfg.UserConfig.Gui.ScrollPastBottom {
		_, h := size(m.dims()[mainView])
		reserved = h - 2
	}
	if m.mainOrigin+reserved >= m.mainLineCount() {
		return
	}

	m.mainOrigin += lines
}

func (m *Model) listRows(p *panel) int {
	d, ok := m.dims()[p.windowName()]
	if !ok {
		return 0
	}

	_, h := size(d)

	return h - 2
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

	return view
}
