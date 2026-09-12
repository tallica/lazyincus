package gui

import (
	"os"
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
	ErrorChan     chan error
	Views         Views

	// if we've suspended the gui (e.g. because we've switched to a subprocess)
	// we typically want to pause some things that are running like background
	// refreshes
	PauseBackgroundThreads bool

	Mutexes

	Panels Panels
}

type Panels struct {
	Instances *panels.SideListPanel[*commands.Instance]
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

	// Maintains the state of manual filtering i.e. typing in a substring
	// to filter on in the current panel.
	Filter filterState
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
func NewGui(log *logrus.Entry, incusCommand *commands.IncusCommand, oSCommand *commands.OSCommand, tr *i18n.TranslationSet, config *config.AppConfig, errorChan chan error) (*Gui, error) {
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
		ErrorChan:     errorChan,
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
		for range ticker.C {
			if !gui.PauseBackgroundThreads {
				_ = function()
			}
		}
	}()
}

// Run sets up the gui with keybindings and starts the mainloop
func (gui *Gui) Run() error {
	defer gui.taskManager.Close()

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
	})
	if err != nil {
		return err
	}
	defer g.Close()

	if !gui.Config.UserConfig.Gui.IgnoreMouseEvents {
		g.Mouse = true
	}

	gui.g = g

	deadlock.Opts.LogBuf = lcUtils.NewOnceWriter(os.Stderr, func() {
		gui.g.Close()
	})

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	g.SetManager(gocui.ManagerFunc(gui.layout), gocui.ManagerFunc(gui.getFocusLayout()))

	if err := gui.createAllViews(); err != nil {
		return err
	}
	if err := gui.setInitialViewContent(); err != nil {
		return err
	}

	gui.setPanels()

	if err = gui.keybindings(g); err != nil {
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
		if err := gui.refreshInstances(); err != nil {
			gui.Log.Error(err)
		}

		gui.goEvery(time.Millisecond*30, gui.reRenderMain)
		gui.goEvery(time.Second, gui.updateInstanceDetails)
		gui.goEvery(time.Second*2, gui.refreshInstancesQuiet)
		gui.goEvery(time.Second*2, gui.configReloader())
	}()

	err = g.MainLoop()
	if err == gocui.ErrQuit {
		return nil
	}
	return err
}

func (gui *Gui) setPanels() {
	gui.Panels = Panels{
		Instances: gui.getInstancesPanel(),
		Menu:      gui.getMenuPanel(),
	}
}

func (gui *Gui) reRenderMain() error {
	mainView := gui.Views.Main
	if mainView == nil {
		return nil
	}
	if mainView.IsTainted() {
		gui.g.Update(func(g *gocui.Gui) error {
			return nil
		})
	}
	return nil
}

func (gui *Gui) updateInstanceDetails() error {
	gui.IncusCommand.RefreshInstanceDetails(gui.Panels.Instances.List.GetAllItems())
	return nil
}

// refreshInstancesQuiet drives the background poll (Incus has no event
// stream to subscribe to). It also redraws the footer on every tick,
// whether or not the refresh succeeded - that's the only place
// IsConnected() is checked, and the footer is otherwise drawn once at
// startup.
func (gui *Gui) refreshInstancesQuiet() error {
	if err := gui.refreshInstances(); err != nil {
		gui.Log.Warn(err)
	}
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
// the point of use. screenMode and language stay as they were - see
// docs/Config.md.
func (gui *Gui) reloadConfig() error {
	if err := gui.Config.ReloadUserConfig(); err != nil {
		return err
	}

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	gui.styleAllViews()
	gui.g.Mouse = !gui.Config.UserConfig.Gui.IgnoreMouseEvents

	return gui.Panels.Instances.RerenderList()
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
	return "instances"
}

func (gui *Gui) IgnoreStrings() []string {
	return gui.Config.UserConfig.Ignore
}

func (gui *Gui) Update(f func() error) {
	gui.g.Update(func(*gocui.Gui) error { return f() })
}

// this is used by our cheatsheet code to generate keybindings.
func (gui *Gui) SetupFakeGui() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
		Headless:         true,
	})
	if err != nil {
		panic(err)
	}
	gui.g = g
	defer g.Close()
	if err := gui.createAllViews(); err != nil {
		panic(err)
	}

	gui.setPanels()
}
