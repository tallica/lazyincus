package gui

import (
	"sort"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/gui/panels"
)

// Binding - a keybinding mapping a key and modifier to a handler. The keypress
// is only handled if the given view has focus, or handled globally if the view
// is ""
type Binding struct {
	ViewName    string
	Handler     func(*gocui.Gui, *gocui.View) error
	Key         any // a rune or a gocui.Key
	Modifier    gocui.Modifier
	Description string
}

// keyLabels names the non-printing keys the keybinding menu can list.
var keyLabels = map[gocui.Key]string{
	gocui.KeyTab:        "tab",
	gocui.KeyBacktab:    "shift+tab",
	gocui.KeyEsc:        "esc",
	gocui.KeyEnter:      "enter",
	gocui.KeySpace:      "space",
	gocui.KeyArrowRight: "►",
	gocui.KeyArrowLeft:  "◄",
	gocui.KeyArrowUp:    "▲",
	gocui.KeyArrowDown:  "▼",
	gocui.KeyPgup:       "PgUp",
	gocui.KeyPgdn:       "PgDn",
}

// GetKey is the binding's key as the keybinding menu shows it, empty for a
// key it has no label for.
func (b *Binding) GetKey() string {
	switch key := b.Key.(type) {
	case rune:
		if key == ' ' {
			return keyLabels[gocui.KeySpace]
		}

		return string(key)
	case gocui.Key:
		return keyLabels[key]
	}

	return ""
}

// GetInitialKeybindings is a function.
func (gui *Gui) GetInitialKeybindings() []*Binding {
	bindings := []*Binding{
		{
			ViewName: "",
			Key:      gocui.KeyEsc,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.escape),
		},
		{
			ViewName: "",
			Key:      'q',
			Modifier: gocui.ModNone,
			Handler:  gui.quit,
		},
		{
			ViewName: "",
			Key:      gocui.KeyCtrlC,
			Modifier: gocui.ModNone,
			Handler:  gui.quit,
		},
		{
			ViewName: "",
			Key:      gocui.KeyPgup,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollUpMain),
		},
		{
			ViewName: "",
			Key:      gocui.KeyPgdn,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollDownMain),
		},
		{
			ViewName: "",
			Key:      gocui.KeyCtrlU,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollUpMain),
		},
		{
			ViewName: "",
			Key:      gocui.KeyCtrlD,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollDownMain),
		},
		{
			ViewName: "",
			Key:      gocui.KeyEnd,
			Modifier: gocui.ModNone,
			Handler:  gui.autoScrollMain,
		},
		{
			ViewName: "",
			Key:      gocui.KeyHome,
			Modifier: gocui.ModNone,
			Handler:  gui.jumpToTopMain,
		},
		{
			ViewName: "",
			Key:      'x',
			Modifier: gocui.ModNone,
			Handler:  gui.handleCreateOptionsMenu,
		},
		{
			ViewName: "",
			Key:      '?',
			Modifier: gocui.ModNone,
			Handler:  gui.handleCreateOptionsMenu,
		},
		{
			ViewName: "menu",
			Key:      gocui.KeyEsc,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.handleMenuClose),
		},
		{
			ViewName: "menu",
			Key:      'q',
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.handleMenuClose),
		},
		{
			// gocui's tcell driver reports the spacebar as KeySpace with no
			// rune, so a ' ' rune binding never matches it.
			ViewName: "menu",
			Key:      gocui.KeySpace,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.handleMenuPress),
		},
		{
			ViewName: "menu",
			Key:      gocui.KeyEnter,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.handleMenuPress),
		},
		{
			ViewName: "menu",
			Key:      'y',
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.handleMenuPress),
		},
		{
			ViewName:    "instances",
			Key:         'S',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceStart),
			Description: gui.Tr.Start,
		},
		{
			ViewName:    "instances",
			Key:         's',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceStop),
			Description: gui.Tr.Stop,
		},
		{
			ViewName:    "instances",
			Key:         'r',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceRestart),
			Description: gui.Tr.Restart,
		},
		{
			ViewName:    "instances",
			Key:         'p',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instancePauseResume),
			Description: gui.Tr.Pause,
		},
		{
			ViewName:    "instances",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "instances",
			Key:         'e',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleHideStoppedInstances,
			Description: gui.Tr.HideStopped,
		},
		{
			ViewName:    "instances",
			Key:         'n',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.snapshotCreatePrompt),
			Description: gui.Tr.NewSnapshot,
		},
		{
			ViewName:    "instances",
			Key:         'm',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleInstanceViewLogs,
			Description: gui.Tr.ViewLogs,
		},
		{
			ViewName:    "instances",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "instances",
			Key:         'a',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceAttachConsole),
			Description: gui.Tr.Attach,
		},
		{
			ViewName:    "instances",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceEdit),
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "instances",
			Key:         'E',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Instances, gui.instanceExecShell),
			Description: gui.Tr.ExecShell,
		},
		{
			ViewName:    "images",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Images, gui.imageDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "snapshots",
			Key:         'n',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleSnapshotCreate,
			Description: gui.Tr.NewSnapshot,
		},
		{
			ViewName:    "snapshots",
			Key:         'e',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleToggleAllSnapshots,
			Description: gui.Tr.ToggleAllSnapshots,
		},
		{
			ViewName:    "snapshots",
			Key:         'r',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Snapshots, gui.snapshotRestore),
			Description: gui.Tr.RestoreSnapshotShort,
		},
		{
			ViewName:    "snapshots",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Snapshots, gui.snapshotDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "images",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Images, gui.imageEdit),
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "volumes",
			Key:         'n',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Volumes, gui.volumeSnapshotCreatePrompt),
			Description: gui.Tr.NewSnapshot,
		},
		{
			ViewName:    "volumes",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Volumes, gui.volumeEdit),
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "networks",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Networks, gui.networkEdit),
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "images",
			Key:         'u',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Images, gui.showImageUsers),
			Description: gui.Tr.ShowUsers,
		},
		{
			ViewName:    "volumes",
			Key:         'u',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Volumes, gui.showVolumeUsers),
			Description: gui.Tr.ShowUsers,
		},
		{
			ViewName:    "networks",
			Key:         'u',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Networks, gui.showNetworkUsers),
			Description: gui.Tr.ShowUsers,
		},
		{
			ViewName:    "images",
			Key:         'D',
			Modifier:    gocui.ModNone,
			Handler:     gui.handlePruneImages,
			Description: gui.Tr.PruneImages,
		},
		{
			ViewName:    "networks",
			Key:         'e',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleToggleUnmanagedNetworks,
			Description: gui.Tr.HideUnmanagedNetworks,
		},
		{
			ViewName:    "profiles",
			Key:         'u',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Profiles, gui.showProfileUsers),
			Description: gui.Tr.ShowUsers,
		},
		{
			ViewName:    "profiles",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Profiles, gui.profileEdit),
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "profiles",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Profiles, gui.profileDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "images",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Images, gui.imageCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "volumes",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Volumes, gui.volumeCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "networks",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Networks, gui.networkCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "profiles",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Profiles, gui.profileCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "snapshots",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Snapshots, gui.snapshotCopy),
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "volumes",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Volumes, gui.volumeDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "networks",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     onSelected(gui.Panels.Networks, gui.networkDelete),
			Description: gui.Tr.Remove,
		},
		{
			ViewName:    "main",
			Key:         gocui.KeyEsc,
			Modifier:    gocui.ModNone,
			Handler:     gui.handleExitMain,
			Description: gui.Tr.Return,
		},
		{
			ViewName: "main",
			Key:      gocui.KeyArrowLeft,
			Modifier: gocui.ModNone,
			Handler:  gui.scrollLeftMain,
		},
		{
			ViewName: "main",
			Key:      gocui.KeyArrowRight,
			Modifier: gocui.ModNone,
			Handler:  gui.scrollRightMain,
		},
		{
			ViewName: "main",
			Key:      'h',
			Modifier: gocui.ModNone,
			Handler:  gui.scrollLeftMain,
		},
		{
			ViewName: "main",
			Key:      'l',
			Modifier: gocui.ModNone,
			Handler:  gui.scrollRightMain,
		},
		{
			ViewName: "filter",
			Key:      gocui.KeyEnter,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.commitFilter),
		},
		{
			ViewName: "filter",
			Key:      gocui.KeyEsc,
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.escapeFilterPrompt),
		},
		{
			ViewName: "",
			Key:      'J',
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollDownMain),
		},
		{
			ViewName: "",
			Key:      'K',
			Modifier: gocui.ModNone,
			Handler:  wrappedHandler(gui.scrollUpMain),
		},
		{
			ViewName: "",
			Key:      'H',
			Modifier: gocui.ModNone,
			Handler:  gui.scrollLeftMain,
		},
		{
			ViewName: "",
			Key:      'L',
			Modifier: gocui.ModNone,
			Handler:  gui.scrollRightMain,
		},
		{
			ViewName:    "",
			Key:         'o',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleOpenConfig,
			Description: gui.Tr.OpenConfig,
		},
		{
			ViewName:    "",
			Key:         'O',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleEditConfig,
			Description: gui.Tr.EditConfig,
		},
		{
			ViewName:    "",
			Key:         'P',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleSwitchProject,
			Description: gui.Tr.SwitchProject,
		},
		{
			ViewName:    "",
			Key:         '+',
			Handler:     wrappedHandler(gui.nextScreenMode),
			Description: gui.Tr.LcNextScreenMode,
		},
		{
			ViewName:    "",
			Key:         '_',
			Handler:     wrappedHandler(gui.prevScreenMode),
			Description: gui.Tr.LcPrevScreenMode,
		},
		{
			ViewName:    "",
			Key:         '=',
			Handler:     wrappedHandler(gui.toggleExpandSidePanel),
			Description: gui.Tr.LcToggleExpandSidePanel,
		},
	}

	setUpDownClickBindings := func(viewName string, onUp func() error, onDown func() error, onClick func() error) {
		bindings = append(bindings, []*Binding{
			{ViewName: viewName, Key: 'k', Modifier: gocui.ModNone, Handler: wrappedHandler(onUp)},
			{ViewName: viewName, Key: gocui.KeyArrowUp, Modifier: gocui.ModNone, Handler: wrappedHandler(onUp)},
			{ViewName: viewName, Key: gocui.MouseWheelUp, Modifier: gocui.ModNone, Handler: wrappedHandler(onUp)},
			{ViewName: viewName, Key: 'j', Modifier: gocui.ModNone, Handler: wrappedHandler(onDown)},
			{ViewName: viewName, Key: gocui.KeyArrowDown, Modifier: gocui.ModNone, Handler: wrappedHandler(onDown)},
			{ViewName: viewName, Key: gocui.MouseWheelDown, Modifier: gocui.ModNone, Handler: wrappedHandler(onDown)},
			{ViewName: viewName, Key: gocui.MouseLeft, Modifier: gocui.ModNone, Handler: wrappedHandler(onClick)},
		}...)
	}

	bindings = append(bindings,
		&Binding{
			ViewName:    "",
			Key:         gocui.KeyTab,
			Modifier:    gocui.ModNone,
			Handler:     wrappedHandler(gui.cycleSidePanel(1)),
			Description: gui.Tr.NextPanel,
		},
		&Binding{
			ViewName:    "",
			Key:         gocui.KeyBacktab,
			Modifier:    gocui.ModNone,
			Handler:     wrappedHandler(gui.cycleSidePanel(-1)),
			Description: gui.Tr.PrevPanel,
		},
	)

	// Arrows and h/l step list by list, a shared window's lists included,
	// the way lazydocker's do; the main panel binds them itself, to scroll
	// sideways, and a view's own binding comes first.
	for _, key := range []any{gocui.KeyArrowLeft, 'h'} {
		bindings = append(bindings, &Binding{
			ViewName:    "",
			Key:         key,
			Handler:     wrappedHandler(gui.cycleSideView(-1)),
			Description: gui.Tr.PreviousList,
		})
	}

	for _, key := range []any{gocui.KeyArrowRight, 'l'} {
		bindings = append(bindings, &Binding{
			ViewName:    "",
			Key:         key,
			Handler:     wrappedHandler(gui.cycleSideView(1)),
			Description: gui.Tr.NextList,
		})
	}

	bindings = append(bindings, gui.stacksKeybindings()...)
	bindings = append(bindings, gui.servicesKeybindings()...)

	for index, window := range gui.sideWindowNames() {
		bindings = append(bindings, &Binding{
			Handler:     gui.handleGoToWindow(window),
			Key:         focusKey(index),
			Description: gui.focusPanelDescription(gui.windowTitle(window)),
		})
	}

	for _, panel := range gui.allListPanels() {
		setUpDownClickBindings(panel.GetView().Name(), panel.HandlePrevLine, panel.HandleNextLine, panel.HandleClick)
	}

	setUpDownClickBindings("main", gui.scrollUpMain, gui.scrollDownMain, gui.handleMainClick)

	for _, panel := range gui.allSidePanels() {
		bindings = append(bindings,
			&Binding{
				ViewName:    panel.GetView().Name(),
				Key:         gocui.KeyEnter,
				Modifier:    gocui.ModNone,
				Handler:     gui.handleEnterMain,
				Description: gui.Tr.FocusMain,
			},
			&Binding{
				ViewName:    panel.GetView().Name(),
				Key:         '[',
				Modifier:    gocui.ModNone,
				Handler:     wrappedHandler(panel.HandlePrevMainTab),
				Description: gui.Tr.PreviousContext,
			},
			&Binding{
				ViewName:    panel.GetView().Name(),
				Key:         ']',
				Modifier:    gocui.ModNone,
				Handler:     wrappedHandler(panel.HandleNextMainTab),
				Description: gui.Tr.NextContext,
			},
		)
	}

	for _, panel := range gui.allListPanels() {
		if !panel.IsFilterDisabled() {
			bindings = append(bindings, &Binding{
				ViewName:    panel.GetView().Name(),
				Key:         '/',
				Modifier:    gocui.ModNone,
				Handler:     wrappedHandler(gui.handleOpenFilter),
				Description: gui.Tr.LcFilter,
			})
		}
	}

	return bindings
}

func (gui *Gui) keybindings(g *gocui.Gui) error {
	bindings := gui.GetInitialKeybindings()

	for _, binding := range bindings {
		if err := g.SetKeybinding(binding.ViewName, binding.Key, binding.Modifier, binding.Handler); err != nil {
			return err
		}
	}

	if err := g.SetTabClickBinding("main", gui.onMainTabClick); err != nil {
		return err
	}

	for _, window := range gui.sideWindowNames() {
		defs := gui.windowDefs(window)
		if len(defs) < 2 {
			continue
		}

		for _, def := range defs {
			if err := g.SetTabClickBinding(def.name, gui.onWindowTabClick(window)); err != nil {
				return err
			}
		}
	}

	return nil
}

// onSelected binds a key to an action on the panel's selected item. With
// nothing selected, the key does nothing.
func onSelected[T comparable](panel *panels.SideListPanel[T], action func(T) error) func(*gocui.Gui, *gocui.View) error {
	return func(*gocui.Gui, *gocui.View) error {
		item, err := panel.GetSelectedItem()
		if err != nil {
			return nil
		}

		return action(item)
	}
}

func wrappedHandler(f func() error) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return f()
	}
}

// stacksKeybindings is the Stacks panel's keys: adding and removing a stack,
// then the Services panel's compose verbs, in its order, over the whole
// stack.
func (gui *Gui) stacksKeybindings() []*Binding {
	binding := func(key rune, handler func(*gocui.Gui, *gocui.View) error, description string) *Binding {
		return &Binding{ViewName: "stacks", Key: key, Modifier: gocui.ModNone, Handler: handler, Description: description}
	}

	return []*Binding{
		binding('a', gui.handleStackAdd, gui.Tr.AddStack),
		binding('D', onSelected(gui.Panels.Stacks, gui.stackRemove), gui.Tr.RemoveStack),
		binding('u', gui.onStackTarget(gui.composeUp), gui.Tr.ComposeUp),
		binding('d', gui.onStackTarget(gui.composeDownMenu), gui.Tr.ComposeDown),
		binding('U', gui.onStackTarget(gui.composeUpPullRecreate), gui.Tr.ComposeUpPullRecreate),
		binding('S', gui.onStackTarget(gui.composeVerb("", "start")), gui.Tr.Start),
		binding('s', gui.onStackTarget(gui.composeVerb(gui.Tr.ConfirmComposeStop, "stop")), gui.Tr.Stop),
		binding('r', gui.onStackTarget(gui.composeVerb("", "restart")), gui.Tr.Restart),
		binding('p', gui.onStack(gui.stackPause), gui.Tr.Pause),
		binding('f', gui.onStackTarget(gui.composeVerb(gui.Tr.ConfirmComposeKill, "kill")), gui.Tr.ComposeKill),
		binding('b', gui.onStackTarget(gui.composeVerb("", "build")), gui.Tr.ComposeBuild),
		binding('g', gui.onStackTarget(gui.composeVerb("", "pull")), gui.Tr.ComposePull),
		binding('m', gui.handleStackViewLogs, gui.Tr.ViewLogs),
		binding('C', gui.onStack(gui.stackMenu), gui.Tr.ComposeProjectActions),
	}
}

// servicesKeybindings is the Services panel's own keys, in the order the

// keybinding menu lists them - which depends on the selected row. A
// service's own row leads with the compose verbs the panel is there for;
// a replica's row leads with the instances panel's keys, in the instances
// panel's order, those being what they do there.
func (gui *Gui) servicesKeybindings() []*Binding {
	bindings := []*Binding{
		{
			ViewName:    "services",
			Key:         'S',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeStart,
			Description: gui.Tr.Start,
		},
		{
			ViewName:    "services",
			Key:         's',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeStop,
			Description: gui.Tr.Stop,
		},
		{
			ViewName:    "services",
			Key:         'r',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeRestart,
			Description: gui.Tr.Restart,
		},
		{
			ViewName:    "services",
			Key:         'p',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposePause,
			Description: gui.Tr.Pause,
		},
		{
			ViewName:    "services",
			Key:         'd',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeDown,
			Description: gui.composeRowDescription(gui.Tr.ComposeDown, gui.Tr.Remove),
		},
		{
			ViewName:    "services",
			Key:         'f',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeKill,
			Description: gui.composeRowDescription(gui.Tr.ComposeKill, gui.Tr.ForceStop),
		},
		{
			ViewName:    "services",
			Key:         'n',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleServiceSnapshotCreate,
			Description: gui.Tr.NewSnapshot,
		},
		{
			ViewName:    "services",
			Key:         'm',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleServiceViewLogs,
			Description: gui.Tr.ViewLogs,
		},
		{
			ViewName:    "services",
			Key:         'y',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleServiceCopy,
			Description: gui.Tr.Copy,
		},
		{
			ViewName:    "services",
			Key:         'c',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleServiceEdit,
			Description: gui.Tr.EditInEditor,
		},
		{
			ViewName:    "services",
			Key:         'E',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleServiceExecShell,
			Description: gui.Tr.ExecShell,
		},
		{
			ViewName:    "services",
			Key:         'u',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeUp,
			Description: gui.serviceScopedDescription(gui.Tr.ComposeUp),
		},
		{
			ViewName:    "services",
			Key:         'U',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeUpPullRecreate,
			Description: gui.serviceScopedDescription(gui.Tr.ComposeUpPullRecreate),
		},
		{
			ViewName:    "services",
			Key:         'b',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeBuild,
			Description: gui.serviceScopedDescription(gui.Tr.ComposeBuild),
		},
		{
			ViewName:    "services",
			Key:         'g',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposePull,
			Description: gui.serviceScopedDescription(gui.Tr.ComposePull),
		},
		{
			ViewName:    "services",
			Key:         'C',
			Modifier:    gocui.ModNone,
			Handler:     gui.handleComposeProjectMenu,
			Description: gui.Tr.ComposeProjectActions,
		},
	}

	order := []rune{'u', 'd', 'U', 'S', 's', 'r', 'p', 'f', 'b', 'g', 'C', 'm', 'n', 'E', 'y'}
	if row, ok := gui.selectedServiceRow(); ok && row.Instance != nil {
		order = []rune{'S', 's', 'r', 'p', 'd', 'f', 'n', 'm', 'y', 'E', 'u', 'U', 'b', 'g', 'C'}
	}

	return orderByKey(bindings, order)
}

// orderByKey lists bindings in the given key order, leaving anything the
// order doesn't name at the end in the order it was declared: a key added
// without touching the order is then listed last rather than dropped.
func orderByKey(bindings []*Binding, order []rune) []*Binding {
	rank := make(map[rune]int, len(order))
	for index, key := range order {
		rank[key] = index
	}

	bindingRank := func(binding *Binding) int {
		key, isRune := binding.Key.(rune)
		if !isRune {
			return len(rank)
		}

		if index, named := rank[key]; named {
			return index
		}

		return len(rank)
	}

	sort.SliceStable(bindings, func(i, j int) bool {
		return bindingRank(bindings[i]) < bindingRank(bindings[j])
	})

	return bindings
}
