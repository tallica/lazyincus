package gui

import (
	"errors"
	"os"
	"os/exec"
	"sync"
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

	// composeProject is the selected stack's project as the daemon holds
	// it, backing the healthcheck line of the Info tabs. Refreshed with the
	// services; atomic, the tabs rendering off the main loop.
	composeProject atomic.Pointer[commands.ComposeProject]

	// composeInstances is the selected stack's services and orphans as the
	// last services refresh found them, for the stack's Info tab.
	composeInstances atomic.Pointer[stackInstances]

	// stackDirs is each listed stack's directory by its remote and project,
	// for the tabs naming a compose instance's stack. See setStackDirs.
	stackDirs atomic.Pointer[map[string]string]

	// selectedStack is the stack the services panel follows: the Stacks
	// panel's selection, set on the main loop and read by fetchServices off
	// it. See followStack.
	selectedStack atomic.Pointer[commands.ComposeStack]

	// localStackDir is where the local stack comes from: -P's directory, or
	// else the working directory, where a missing compose file means no
	// local stack rather than a row showing the error. Set before run.
	localStackDir      string
	localStackExplicit bool

	// loadStack reads a stack's compose config; tests stand in for
	// incus-compose here.
	loadStack func(dir string) *commands.ComposeStack
	// loadBackups lists a stack's backups through command; tests stand in
	// for incus-compose here too.
	loadBackups func(command *commands.IncusCommand, dir string) ([]*commands.ComposeBackup, error)
	stacks      stackCache

	// remotes are the commands for stacks on other remotes than the
	// session's, and stacksElsewhere whether any is listed: the stacks
	// poller's copy of State.StacksElsewhere.
	remotes         remoteCommands
	stacksElsewhere atomic.Bool

	// home is what the Stacks panel shortens paths against.
	home string

	// stopped closes when run returns, stopping the pollers.
	stopped chan struct{}

	events        eventBatch
	eventsRescope chan struct{}
	// eventsLive is whether an event stream is open.
	eventsLive atomic.Bool
	// watching is watchEvents, which run waits out.
	watching sync.WaitGroup
	// operations is the session's operation history, main loop only, and
	// operationCounts its running and unseen failed counts, packed, for the
	// footer, which is drawn off it too.
	operations      operationLog
	operationCounts atomic.Int64

	// networkTabs counts the forward and ACL events, which change what a
	// network's tabs show without changing the network.
	networkTabs atomic.Uint64
}

type Panels struct {
	Instances  *panels.SideListPanel[*commands.Instance]
	Images     *panels.SideListPanel[*commands.Image]
	Snapshots  *panels.SideListPanel[*commands.Snapshot]
	Backups    *panels.SideListPanel[*commands.ComposeBackup]
	Volumes    *panels.SideListPanel[*commands.Volume]
	Networks   *panels.SideListPanel[*commands.Network]
	Profiles   *panels.SideListPanel[*commands.Profile]
	Operations *panels.SideListPanel[*commands.Operation]
	Services   *panels.SideListPanel[*commands.ServiceRow]
	Stacks     *panels.SideListPanel[*commands.ComposeStack]
	Menu       *panels.SideListPanel[*types.MenuItem]
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

	// ShowStackInstances puts the listed stacks' compose instances back in
	// the instances panel, alongside the Services panel's rows for them.
	ShowStackInstances bool

	// ShowUnmanagedNetworks lists the host interfaces Incus merely reports
	// alongside its own networks.
	ShowUnmanagedNetworks bool

	ScreenMode WindowMaximisation

	// Seeded from expandFocusedSidePanel and then owned by the session, so
	// '=' outlives a config reload.
	ExpandSidePanel bool

	// Maintains the state of manual filtering i.e. typing in a substring
	// to filter on in the current panel.
	Filter filterState

	Connection connectionState

	// ComposeAvailable is whether incus-compose is on PATH, which the Stacks
	// and Services panels need to be there at all. Resolved once at
	// startup: panel visibility is fixed for the session.
	ComposeAvailable bool

	// StackServices are the services each listed stack on the session's
	// remote declares, by its Incus project: their instances are the services
	// panel's rather than the instances panel's.
	StackServices map[string]map[string]bool

	// SnapshotsInstances are the instances the snapshots panel is showing
	// the snapshots of, and SnapshotsLabel what its title calls them:
	// whatever the list you were last in had selected, which is every
	// replica of a selected service - see refreshSnapshotsFor.
	SnapshotsInstances []*commands.Instance
	SnapshotsLabel     string
	// SnapshotsVolume is the key of the custom volume the panel follows
	// instead, when the volumes panel's selection is one.
	SnapshotsVolume string

	// ActiveWindowViews is the view each shared window shows, by window -
	// whichever was focused in it last. See window.go.
	ActiveWindowViews map[string]string

	// InstanceUsers narrows the instances panel to what uses one resource,
	// nil for the usual list. See showUsers.
	InstanceUsers *instanceUsers

	// Seeded from showAllSnapshots and then owned by the session, as
	// ExpandSidePanel is.
	SnapshotsShowAll bool

	// What the snapshots panel's rows span as of its last render, which
	// decides whether a row names its instance and project.
	SnapshotsSpan snapshotsSpan

	// OperationsSeenAt is when the Operations tab last had focus: the
	// footer counts the failures since.
	OperationsSeenAt time.Time

	// BackupVerifications are `backup verify`'s reports, by backupKey:
	// verifying walks every restore point, so it's asked for, not polled,
	// and a report outlives the refreshes after it.
	BackupVerifications map[string]*commands.BackupVerification

	// Whether each panel's current contents span more than one project, and
	// so need a project column to stay unambiguous. Recomputed on refresh:
	// the all-projects view of a server with a single project reads better
	// without a column repeating that project on every row.
	SpansProjects spansProjects

	// ServicesNote is what the services list says while it's empty for want
	// of the stack's remote answering. Main loop only.
	ServicesNote string

	// StacksElsewhere is whether a stack listed is on a remote other than
	// the session's, which the stacks then need a remote column for.
	StacksElsewhere bool

	// StacksHere is whether a stack listed is on the session's remote;
	// without one, Stacks and Services collapse while unfocused.
	StacksHere bool

	// Landing is set from startup or a remote switch until the stacks are
	// next read, which then decides whether the focus leaves them.
	Landing bool
}

type snapshotsSpan struct {
	Instances bool
	Projects  bool
}

type spansProjects struct {
	Instances  bool
	Images     bool
	Volumes    bool
	Networks   bool
	Profiles   bool
	Operations bool
}

// projectColumns is how many columns a project column puts ahead of the
// rest: one when the panel spans projects.
func projectColumns(spans bool) int {
	if spans {
		return 1
	}

	return 0
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

var deadlockOptions sync.Once

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
		SnapshotsShowAll:     config.UserConfig.Gui.ShowAllSnapshots,
		StacksHere:           true,
		Landing:              true,
	}

	home, _ := os.UserHomeDir()

	gui := &Gui{
		home:          home,
		loadStack:     incusCommand.LoadComposeStack,
		loadBackups:   (*commands.IncusCommand).ComposeBackups,
		Log:           log,
		IncusCommand:  incusCommand,
		OSCommand:     oSCommand,
		State:         initialState,
		Config:        config,
		Tr:            tr,
		statusManager: &statusManager{},
		taskManager:   tasks.NewTaskManager(log, tr),
		stopped:       make(chan struct{}),
		eventsRescope: make(chan struct{}, 1),
		remotes: remoteCommands{
			connect: incusCommand.ConnectRemote,
			known:   incusCommand.IsInstanceRemote,
			names:   incusCommand.InstanceRemoteNames,
		},
	}

	// The options are the process's: a later Gui - a test's - rewriting them
	// would race an earlier one's goroutines still taking locks.
	deadlockOptions.Do(func() {
		deadlock.Opts.Disable = !gui.Config.Debug
		deadlock.Opts.DeadlockTimeout = 10 * time.Second
	})

	gui.remotes.connected = gui.refreshForRemote

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

// watchedPollInterval is how often a list the event stream keeps current is
// polled while the stream is open (docs/Incus.md, "Events").
const watchedPollInterval = time.Minute

// pollUnlessWatched is goEvery for a list the event stream keeps current.
func (gui *Gui) pollUnlessWatched(interval time.Duration, function func() error) {
	gui.pollWhileUnwatched(interval, function, func() bool { return false })
}

// pollWhileUnwatched is pollUnlessWatched for a list the stream keeps
// current only while unwatched says nothing in it is beyond the stream.
func (gui *Gui) pollWhileUnwatched(interval time.Duration, function func() error, unwatched func() bool) {
	var last time.Time

	gui.goEvery(interval, func() error {
		if gui.eventsLive.Load() && !unwatched() && time.Since(last) < watchedPollInterval {
			return nil
		}

		last = time.Now()

		return function()
	})
}

// Run sets up the gui with keybindings and starts the mainloop
func (gui *Gui) Run() error {
	// Before any view exists: it decides whether the Stacks and Services
	// panels are there at all, and so which panels get numbered and focused.
	if _, err := exec.LookPath("incus-compose"); err == nil {
		gui.State.ComposeAvailable = true
	}

	gui.localStackDir, gui.localStackExplicit = localStackDir()

	if err := gui.Config.PinStacks(gui.IncusCommand.RemoteName()); err != nil {
		gui.Log.Warn(err)
	}

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
	defer gui.watching.Wait()
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

	for _, panel := range gui.allSidePanels() {
		panel.Await(gui.Tr.Loading)
	}

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

	gui.watching.Add(1)
	go func() {
		defer gui.watching.Done()
		gui.watchEvents()
	}()

	go func() {
		for _, err := range gui.refreshAll() {
			gui.Log.Error(err)
		}

		gui.goEvery(time.Second*2, gui.refreshInstancesQuiet)
		gui.goEvery(time.Second*2, gui.configReloader())
		gui.pollUnlessWatched(time.Second*10, gui.refreshImagesQuiet)
		gui.pollUnlessWatched(time.Second*10, gui.refreshVolumesQuiet)
		gui.pollUnlessWatched(time.Second*10, gui.refreshBackupsQuiet)
		gui.pollUnlessWatched(time.Second*10, gui.refreshNetworksQuiet)
		gui.pollUnlessWatched(time.Second*10, gui.refreshProfilesQuiet)
		gui.pollUnlessWatched(time.Second*10, gui.refreshOperationsQuiet)
		// The session's stream says nothing of another remote's stacks.
		gui.pollWhileUnwatched(time.Second*10, gui.refreshStacksQuiet, gui.stacksElsewhere.Load)
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

// fetchGroups is every panel's fetch, in groups read side by side. The
// stacks lead theirs: which instances the instances panel leaves out is
// theirs to say, and a compose instance has a row in both of the others.
func (gui *Gui) fetchGroups() [][]fetch {
	return [][]fetch{
		{gui.fetchStacks, gui.fetchInstances, gui.fetchServices},
		{gui.fetchImages},
		{gui.fetchVolumes},
		{gui.fetchNetworks},
		{gui.fetchProfiles},
		{gui.fetchOperations},
	}
}

func (gui *Gui) setPanels() {
	gui.Panels = Panels{
		Instances:  gui.getInstancesPanel(),
		Snapshots:  gui.getSnapshotsPanel(),
		Backups:    gui.getBackupsPanel(),
		Images:     gui.getImagesPanel(),
		Volumes:    gui.getVolumesPanel(),
		Networks:   gui.getNetworksPanel(),
		Profiles:   gui.getProfilesPanel(),
		Operations: gui.getOperationsPanel(),
		Services:   gui.getServicesPanel(),
		Stacks:     gui.getStacksPanel(),
		Menu:       gui.getMenuPanel(),
	}
}

// refreshInstancesQuiet drives the background poll, which events don't
// replace (docs/Incus.md, "Events"): the services too, a service being its
// instances. It also reports on the connection every tick, whether or not
// the refresh succeeded - this is what notices a daemon that has gone away,
// and the footer is otherwise drawn once at startup.
func (gui *Gui) refreshInstancesQuiet() error {
	if err := gui.refreshInstancesAndServices(); err != nil {
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

	if gui.State.InstanceUsers != nil && gui.currentViewName() == "instances" {
		return gui.clearInstanceUsers()
	}

	return nil
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
// the point of use. screenMode, expandFocusedSidePanel, showAllSnapshots and
// language stay as they were - see docs/Config.md.
func (gui *Gui) reloadConfig() error {
	if err := gui.Config.ReloadUserConfig(); err != nil {
		return err
	}

	if err := gui.SetColorScheme(); err != nil {
		return err
	}

	gui.styleAllViews()
	gui.g.Mouse = !gui.Config.UserConfig.Gui.IgnoreMouseEvents

	return gui.rerenderInstanceLists()
}

// rerenderInstanceLists redraws both panels an instance can be a row in.
func (gui *Gui) rerenderInstanceLists() error {
	if err := gui.Panels.Instances.RerenderList(); err != nil {
		return err
	}

	if gui.composeUnavailable() {
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
