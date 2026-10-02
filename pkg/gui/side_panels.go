package gui

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/panels"
)

// sidePanelDef describes one side panel ahead of the panel object existing:
// views are created (and styled) before setPanels builds the panels that
// point at them, so view creation, styling and the number keys all derive
// from this list rather than from allSidePanels.
//
// Order is the order panels appear down the side column, and the order their
// number keys run in - the first panel is `1`, and is what the app focuses on
// startup.
type sidePanelDef struct {
	name    string
	title   string
	viewPtr **gocui.View
	// panel returns the panel object for this definition, once setPanels has
	// built them.
	panel func() panels.ISideListPanel
	// hidden reports whether this panel is absent for the session. Read
	// while styling views and binding keys, both of which happen before
	// setPanels, so it can't go through the panel's own Hide.
	hidden func() bool
	// window is the slot the panel shares with others, as one of its tabs;
	// empty for a panel with a slot of its own. shortTitle is its tab's
	// name when the window's full ones don't fit.
	window     string
	shortTitle string
}

func (def sidePanelDef) windowName() string {
	if def.window == "" {
		return def.name
	}

	return def.window
}

func (gui *Gui) sidePanelDefs() []sidePanelDef {
	return []sidePanelDef{
		{
			name:    "stacks",
			title:   gui.Tr.StacksTitle,
			viewPtr: &gui.Views.Stacks,
			panel:   func() panels.ISideListPanel { return gui.Panels.Stacks },
			hidden:  gui.composeUnavailable,
		},
		{
			name:    "services",
			title:   gui.servicesPanelTitle(),
			viewPtr: &gui.Views.Services,
			panel:   func() panels.ISideListPanel { return gui.Panels.Services },
			hidden:  gui.composeUnavailable,
		},
		{
			name:    "instances",
			title:   gui.instancesPanelTitle(),
			viewPtr: &gui.Views.Instances,
			panel:   func() panels.ISideListPanel { return gui.Panels.Instances },
		},
		{
			name:    "snapshots",
			title:   gui.Tr.SnapshotsTitle,
			viewPtr: &gui.Views.Snapshots,
			panel:   func() panels.ISideListPanel { return gui.Panels.Snapshots },
		},
		{
			name:       "images",
			title:      gui.Tr.ImagesTitle,
			viewPtr:    &gui.Views.Images,
			panel:      func() panels.ISideListPanel { return gui.Panels.Images },
			window:     resourcesWindow,
			shortTitle: gui.Tr.ImagesShort,
		},
		{
			name:       "volumes",
			title:      gui.Tr.VolumesTitle,
			viewPtr:    &gui.Views.Volumes,
			panel:      func() panels.ISideListPanel { return gui.Panels.Volumes },
			window:     resourcesWindow,
			shortTitle: gui.Tr.VolumesShort,
		},
		{
			name:       "networks",
			title:      gui.Tr.NetworksTitle,
			viewPtr:    &gui.Views.Networks,
			panel:      func() panels.ISideListPanel { return gui.Panels.Networks },
			window:     resourcesWindow,
			shortTitle: gui.Tr.NetworksShort,
		},
		{
			name:       "profiles",
			title:      gui.Tr.ProfilesTitle,
			viewPtr:    &gui.Views.Profiles,
			panel:      func() panels.ISideListPanel { return gui.Panels.Profiles },
			window:     resourcesWindow,
			shortTitle: gui.Tr.ProfilesShort,
		},
	}
}

// composeUnavailable hides the Stacks and Services panels: without
// incus-compose there's no reading a compose file, nor acting on one.
func (gui *Gui) composeUnavailable() bool {
	return !gui.State.ComposeAvailable
}

// visibleSidePanelDefs drops the panels this session doesn't have. Both the
// number keys and the layout are numbered over this (by window) rather than
// over every definition, so a hidden panel leaves no gap at `1`.
func (gui *Gui) visibleSidePanelDefs() []sidePanelDef {
	return lo.Filter(gui.sidePanelDefs(), func(def sidePanelDef, _ int) bool {
		return def.hidden == nil || !def.hidden()
	})
}

// focusKey is the key that jumps to the visible window at this index: `1`
// for the first, and so on.
func focusKey(index int) rune {
	return rune('1' + index)
}

func (gui *Gui) sidePanelTitlePrefix(index int) string {
	return fmt.Sprintf("[%c]", focusKey(index))
}

func (gui *Gui) focusPanelDescription(title string) string {
	return fmt.Sprintf(gui.Tr.FocusPanel, strings.ToLower(title))
}

// cycleSidePanel moves focus to the next (offset 1) or previous (offset -1)
// visible side window, wrapping at both ends. It steps from the last side
// panel that had focus, so tabbing out of the main panel continues from the
// list you were last in rather than jumping back to the first.
func (gui *Gui) cycleSidePanel(offset int) func() error {
	return func() error {
		if gui.popupPanelFocused() {
			return nil
		}

		names := gui.sideWindowNames()
		if len(names) == 0 {
			return nil
		}

		index := lo.IndexOf(names, gui.currentSideWindowName())
		if index < 0 {
			index = 0
		}

		next := names[((index+offset)%len(names)+len(names))%len(names)]

		view, err := gui.g.View(gui.activeViewInWindow(next))
		if err != nil {
			return err
		}

		return gui.switchFocus(view)
	}
}

// cycleSideView is cycleSidePanel over every visible side list rather than
// every window, so a shared window's lists are stops of their own.
func (gui *Gui) cycleSideView(offset int) func() error {
	return func() error {
		if gui.popupPanelFocused() {
			return nil
		}

		names := gui.sideViewNames()
		if len(names) == 0 {
			return nil
		}

		index := lo.IndexOf(names, gui.currentSideViewName())
		if index < 0 {
			index = 0
		}

		view, err := gui.g.View(names[((index+offset)%len(names)+len(names))%len(names)])
		if err != nil {
			return err
		}

		return gui.switchFocus(view)
	}
}

// servicesPanelTitle names the compose project the panel acts on. It's the
// same one on every row, so it belongs in the title rather than in a column
// - the reasoning the instances panel's project column already follows.
func (gui *Gui) servicesPanelTitle() string {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return gui.Tr.ServicesTitle
	}

	return fmt.Sprintf(gui.Tr.ServicesTitleProject, gui.onRemote(stack.Name, stack.Remote))
}

// instancesPanelTitle marks the panel as holding what's left once the
// services panel has taken the stacks', the way lazydocker's Containers
// panel becomes "Standalone Containers" alongside its Services panel.
func (gui *Gui) instancesPanelTitle() string {
	if users := gui.State.InstanceUsers; users != nil {
		return fmt.Sprintf(gui.Tr.InstancesUsing, users.label)
	}

	if len(gui.State.StackServices) == 0 || gui.State.ShowStackInstances {
		return gui.Tr.InstancesTitle
	}

	return gui.Tr.StandaloneInstancesTitle
}

func (gui *Gui) allSidePanels() []panels.ISideListPanel {
	return lo.Map(gui.sidePanelDefs(), func(def sidePanelDef, _ int) panels.ISideListPanel {
		return def.panel()
	})
}

func (gui *Gui) allListPanels() []panels.ISideListPanel {
	return append(gui.allSidePanels(), gui.Panels.Menu)
}
