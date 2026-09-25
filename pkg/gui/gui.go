package gui

import (
	"errors"
	"os"
	"sync/atomic"
	"time"

	lcUtils "github.com/jesseduffield/lazycore/pkg/utils"

	"github.com/jesseduffield/gocui"
	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// Gui wraps the gocui Gui object which handles rendering and events
type Gui struct {
	g             *gocui.Gui
	Log           *logrus.Entry
	IncusCommand  *commands.IncusCommand
	OSCommand     *commands.OSCommand
	State         guiState
	Config        *config.AppConfig
	Tr            *i18n.TranslationSet
	statusManager *statusManager
	taskManager   *tasks.TaskManager
	Views         Views

	// mainViewWidth is how wide the main panel currently is, recorded by
	// layout for the render goroutines: a tab's content is built off the
	// main loop, where reading the view's own size would race.
	mainViewWidth atomic.Int32

	// if we've suspended the gui (e.g. because we've switched to a subprocess)
	// we typically want to pause some things that are running like background
	// refreshes
	PauseBackgroundThreads atomic.Bool

	Mutexes

	Panels Panels

	refreshes refreshSeqs

	mainView mainViewState

	// composeProject is the local project as the daemon holds it, backing
	// the healthcheck line of the services panel's Info tab. Refreshed with
	// the services; atomic, the tab rendering off the main loop.
	composeProject atomic.Pointer[commands.ComposeProject]

	// stopped closes when run returns, stopping the pollers.
	stopped chan struct{}
}

type Panels struct {
	Instances *panels.SideListPanel[*commands.Instance]
	Images    *panels.SideListPanel[*commands.Image]
	Snapshots *panels.SideListPanel[*commands.Snapshot]
	Volumes   *panels.SideListPanel[*commands.Volume]
	Networks  *panels.SideListPanel[*commands.Network]
	Services  *panels.SideListPanel[*commands.ServiceRow]
	Menu      *panels.SideListPanel[*types.MenuItem]
}

type Mutexes struct {
	SubprocessMutex deadlock.Mutex
	ViewStackMutex  deadlock.Mutex
}

type mainPanelState struct {
	// ObjectKey tells us what context we are in. For example, if we are
	// looking at the logs of a particular instance this key might be
	// 'instances-<name>-logs'. The key is made so that if something changes
	// which might require us to re-run the logs command or run a different
	// command, the key will be different, and we'll then know to do whatever
	// is required.
	ObjectKey string
}

type panelStates struct {
	Main *mainPanelState
}

type guiState struct {
	// the names of views in the current focus stack (last item is the current view)
	ViewStack []string
	Platform  commands.Platform
	Panels    *panelStates

	// if true, we show instances with a 'Stopped' status in the instances panel
	ShowStoppedInstances bool

	ScreenMode WindowMaximisation

	// Seeded from expandFocusedSidePanel and then owned by the session, so
	// '=' outlives a config reload.
	ExpandSidePanel bool

	// Maintains the state of manual filtering i.e. typing in a substring
	// to filter on in the current panel.
	Filter filterState

	Connection connectionState

	// The compose project whose compose file lives in lazyincus's own
	// working directory - see (*Gui).localComposeProject. Empty if there
	// isn't one, which is what hides the services panel; resolved once at
	// startup, since the working directory doesn't change mid-session.
	LocalComposeProject string

	// ComposeServiceDefs is what that compose file declares, which is what
	// lets a service with nothing running still have a row. Resolved with
	// the project name, from the same output.
	ComposeServiceDefs []commands.ComposeService

	// SnapshotsInstances are the instances the snapshots panel is showing
	// the snapshots of, and SnapshotsLabel what its title calls them:
	// whatever the list you were last in had selected, which is every
	// replica of a selected service - see refreshSnapshotsFor.
	SnapshotsInstances []*commands.Instance
	SnapshotsLabel     string

	// Whether each panel's current contents span more than one project, and
	// so need a project column to stay unambiguous. Recomputed on refresh:
	// the all-projects view of a server with a single project reads better
	// without a column repeating that project on every row.
	SpansProjects spansProjects
}

type spansProjects struct {
	Instances bool
	Images    bool
	Volumes   bool
	Networks  bool
}

// spansMultipleProjects reports whether the given projects include more than
// one distinct non-empty name.
func spansMultipleProjects(projects []string) bool {
	seen := ""

	for _, project := range projects {
		if project == "" {
			continue
		}

		if seen != "" && project != seen {
			return true
		}

		seen = project
	}

	return false
}

type filterState struct {
	// If true then we're either currently inside the filter view
	// or we've committed the filter and we're back in the list view
	active bool
	// The panel that we're filtering.
	panel panels.ISideListPanel
	// The string that we're filtering on
	needle string
}

// screen sizing determines how much space your selected window takes up (window
// as in panel, not your terminal's window).
type WindowMaximisation int

const (
	SCREEN_NORMAL WindowMaximisation = iota
	SCREEN_HALF
	SCREEN_FULL
)

func getScreenMode(config *config.AppConfig) WindowMaximisation {
	switch config.UserConfig.Gui.ScreenMode {
	case "normal":
		return SCREEN_NORMAL
	case "half":
		return SCREEN_HALF
	case "full", "fullscreen":
		return SCREEN_FULL
	default:
		return SCREEN_NORMAL
	}
}

// NewGui builds a new gui handler
func NewGui(log *logrus.Entry, incusCommand *commands.IncusCommand, oSCommand *commands.OSCommand, tr *i18n.TranslationSet, config *config.AppConfig) (*Gui, error) {
	initialState := guiState{
		Platform: *oSCommand.Platform,
		Panels: &panelStates{
			Main: &mainPanelState{
				ObjectKey: "",
			},
		},
		ViewStack: []string{},

		ShowStoppedInstances: true,
		ScreenMode:           getScreenMode(config),
		ExpandSidePanel:      config.UserConfig.Gui.ExpandFocusedSidePanel,
	}

	gui := &Gui{
		Log:           log,
		IncusCommand:  incusCommand,
		OSCommand:     oSCommand,
		State:         initialState,
		Config:        config,
		Tr:            tr,
		statusManager: &statusManager{},
		taskManager:   tasks.NewTaskManager(log, tr),
		stopped:       make(chan struct{}),
	}

	deadlock.Opts.Disable = !gui.Config.Debug
	deadlock.Opts.DeadlockTimeout = 10 * time.Second

	return gui, nil
}

func (gui *Gui) renderGlobalOptions() error {
	return gui.renderOptionsMap(map[string]string{
		"PgUp/PgDn": gui.Tr.Scroll,
		"← → ↑ ↓":   gui.Tr.Navigate,
		"q":         gui.Tr.Quit,
		"x":         gui.Tr.Menu,
	})
}

func (gui *Gui) goEvery(interval time.Duration, function func() error) {
	_ = function()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-gui.stopped:
				return
			case <-ticker.C:
				if !gui.PauseBackgroundThreads.Load() {
					_ = function()
				}
			}
		}
	}()
}

// Run sets up the gui with keybindings and starts the mainloop
func (gui *Gui) Run() error {
	// Before any view exists: whether there's a compose file in the working
	// directory decides whether the services panel is there at all, and so
	// which panels get styled, numbered and focused first. One fast
	// subprocess, once - the working directory doesn't change mid-session.
	gui.State.LocalComposeProject, gui.State.ComposeServiceDefs = gui.localComposeProject()

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
	})
	if err != nil {
		return err
	}

	deadlock.Opts.LogBuf = lcUtils.NewOnceWriter(os.Stderr, func() {
		g.Close()
	})

	return gui.run(g)
}

// run drives a gocui.Gui until quit: all of Run but making it, which a
// test does headless.
func (gui *Gui) run(g *gocui.Gui) error {
	defer gui.taskManager.Close()
	defer g.Close()
	defer close(gui.stopped)

	if !gui.Config.UserConfig.Gui.IgnoreMouseEvents {
		g.Mouse = true
	}

	// Lets a view draw a hint in its bottom border; nothing here uses the
	// list-position footer gocui named the flag after.
	g.ShowListFooter = true

	gui.g = g

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	g.ErrorHandler = gui.handleError

	// A popup has the keyboard, so a click or a wheel outside it does
	// nothing; gocui would otherwise move the cursor of the list under it
	// before any binding of ours could say no.
	g.ShouldHandleMouseEvent = func(v *gocui.View, _ gocui.Key) bool {
		return !gui.popupPanelFocused() || gui.isPopupPanel(v.Name())
	}

	g.SetManager(gocui.ManagerFunc(gui.layout), gocui.ManagerFunc(gui.getFocusLayout()))

	if err := gui.createAllViews(); err != nil {
		return err
	}
	if err := gui.setInitialViewContent(); err != nil {
		return err
	}

	gui.setPanels()

	if err := gui.keybindings(g); err != nil {
		return err
	}

	if gui.g.CurrentView() == nil {
		viewName := gui.initiallyFocusedViewName()
		view, err := gui.g.View(viewName)
		if err != nil {
			return err
		}

		if err := gui.switchFocus(view); err != nil {
			return err
		}
	}

	go func() {
		for _, fetch := range gui.allFetches() {
			if err := gui.refresh(nil, fetch); err != nil {
				gui.Log.Error(err)
			}
		}

		gui.goEvery(time.Second*2, gui.refreshInstancesQuiet)
		gui.goEvery(time.Second*2, gui.configReloader())
		gui.goEvery(time.Second*10, gui.refreshImagesQuiet)
		gui.goEvery(time.Second*10, gui.refreshVolumesQuiet)
		gui.goEvery(time.Second*10, gui.refreshNetworksQuiet)
		gui.goEvery(time.Second*10, gui.refreshServicesQuiet)
	}()

	err := g.MainLoop()
	if errors.Is(err, gocui.ErrQuit) {
		return nil
	}
	return err
}

// handleError keeps a failed keypress or refresh from taking the app down
// with it: gocui ends the main loop on any error out of a keybinding or an
// Update closure, and returning nil here means carry on instead. Quitting
// is unaffected - gocui excludes ErrQuit before consulting this.
func (gui *Gui) handleError(err error) error {
	gui.Log.Error(err)

	// The modal and the footer report an unreachable daemon already.
	if commands.IsConnectionError(err) {
		gui.IncusCommand.NoteError(err)
		return nil
	}

	if err := gui.createErrorPanel(err.Error()); err != nil {
		gui.Log.Error(err)
	}

	return nil
}

// allFetches is every panel's fetch, in the order startup and a project
// switch run them.
func (gui *Gui) allFetches() []fetch {
	return []fetch{gui.fetchInstances, gui.fetchImages, gui.fetchVolumes, gui.fetchNetworks, gui.fetchServices}
}

func (gui *Gui) setPanels() {
	gui.Panels = Panels{
		Instances: gui.getInstancesPanel(),
		Snapshots: gui.getSnapshotsPanel(),
		Images:    gui.getImagesPanel(),
		Volumes:   gui.getVolumesPanel(),
		Networks:  gui.getNetworksPanel(),
		Services:  gui.getServicesPanel(),
		Menu:      gui.getMenuPanel(),
	}
}

// refreshInstancesQuiet drives the background poll (Incus has no event
// stream to subscribe to). It also reports on the connection every tick,
// whether or not the refresh succeeded - this is what notices a daemon that
// has gone away, and the footer is otherwise drawn once at startup.
func (gui *Gui) refreshInstancesQuiet() error {
	if err := gui.refreshInstances(); err != nil {
		gui.Log.Warn(err)
	}

	gui.syncConnection()

	return gui.renderString(gui.g, "information", gui.getInformationContent())
}

func (gui *Gui) quit(g *gocui.Gui, v *gocui.View) error {
	if gui.Config.UserConfig.ConfirmOnQuit {
		return gui.createConfirmationPanel("", gui.Tr.ConfirmQuit, func(g *gocui.Gui, v *gocui.View) error {
			return gocui.ErrQuit
		}, nil)
	}
	return gocui.ErrQuit
}

// this handler is executed when we press escape when there is only one view
// on the stack.
func (gui *Gui) escape() error {
	if gui.State.Filter.active {
		return gui.clearFilter()
	}

	return nil
}

func (gui *Gui) handleDonate(g *gocui.Gui, v *gocui.View) error {
	if !gui.g.Mouse {
		return nil
	}

	cx, _ := v.Cursor()
	if cx > len(gui.Tr.Donate) {
		return nil
	}
	return gui.OSCommand.OpenLink("https://github.com/lxc/incus")
}

func (gui *Gui) editFile(filename string) error {
	cmd, err := gui.OSCommand.EditFile(filename)
	if err != nil {
		return gui.createErrorPanel(err.Error())
	}

	return gui.runSubprocess(cmd)
}

func (gui *Gui) openFile(filename string) error {
	if err := gui.OSCommand.OpenFile(filename); err != nil {
		return gui.createErrorPanel(err.Error())
	}
	return nil
}

func (gui *Gui) handleOpenConfig(g *gocui.Gui, v *gocui.View) error {
	return gui.openFile(gui.Config.ConfigFilename())
}

func (gui *Gui) handleEditConfig(g *gocui.Gui, v *gocui.View) error {
	if err := gui.editFile(gui.Config.ConfigFilename()); err != nil {
		return err
	}

	if err := gui.reloadConfig(); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	return nil
}

// reloadConfig re-applies the settings the app caches rather than reads at
// the point of use. screenMode, expandFocusedSidePanel and language stay as
// they were - see docs/Config.md.
func (gui *Gui) reloadConfig() error {
	if err := gui.Config.ReloadUserConfig(); err != nil {
		return err
	}

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	gui.styleAllViews()
	gui.g.Mouse = !gui.Config.UserConfig.Gui.IgnoreMouseEvents

	if err := gui.Panels.Instances.RerenderList(); err != nil {
		return err
	}

	if gui.noLocalComposeProject() {
		return nil
	}

	return gui.Panels.Services.RerenderList()
}

// configReloader polls the config file's modification time. It covers the
// edits handleEditConfig can't see: 'o' hands the file to an external app,
// and people edit it in other terminals.
func (gui *Gui) configReloader() func() error {
	var lastModTime time.Time

	return func() error {
		info, err := os.Stat(gui.Config.ConfigFilename())
		if err != nil {
			return nil
		}

		modTime := info.ModTime()
		if lastModTime.IsZero() || modTime.Equal(lastModTime) {
			lastModTime = modTime
			return nil
		}
		lastModTime = modTime

		gui.g.Update(func(*gocui.Gui) error {
			if err := gui.reloadConfig(); err != nil {
				// Likely a half-written save; the next write reloads again.
				gui.Log.Warn(err)
			}
			return nil
		})

		return nil
	}
}

func (gui *Gui) ShouldRefresh(key string) bool {
	if gui.State.Panels.Main.ObjectKey == key {
		return false
	}

	gui.State.Panels.Main.ObjectKey = key
	return true
}

func (gui *Gui) initiallyFocusedViewName() string {
	return gui.visibleSidePanelDefs()[0].name
}

func (gui *Gui) IgnoreStrings() []string {
	return gui.Config.UserConfig.Ignore
}

func (gui *Gui) Update(f func() error) {
	gui.g.Update(func(*gocui.Gui) error { return f() })
}
