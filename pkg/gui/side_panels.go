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
}

func (gui *Gui) sidePanelDefs() []sidePanelDef {
	return []sidePanelDef{
		{
			name:    "instances",
			title:   gui.Tr.InstancesTitle,
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
			name:    "images",
			title:   gui.Tr.ImagesTitle,
			viewPtr: &gui.Views.Images,
			panel:   func() panels.ISideListPanel { return gui.Panels.Images },
		},
		{
			name:    "volumes",
			title:   gui.Tr.VolumesTitle,
			viewPtr: &gui.Views.Volumes,
			panel:   func() panels.ISideListPanel { return gui.Panels.Volumes },
		},
		{
			name:    "networks",
			title:   gui.Tr.NetworksTitle,
			viewPtr: &gui.Views.Networks,
			panel:   func() panels.ISideListPanel { return gui.Panels.Networks },
		},
	}
}

// focusKey is the key that jumps to the panel at this index: `1` for the
// first, and so on. Panels keep their key when another one is hidden, so the
// keys don't shuffle under the user.
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
// visible side panel, wrapping at both ends. It steps from the last side
// panel that had focus, so tabbing out of the main panel continues from the
// list you were last in rather than jumping back to the first.
func (gui *Gui) cycleSidePanel(offset int) func() error {
	return func() error {
		if gui.popupPanelFocused() {
			return nil
		}

		names := gui.sideViewNames()
		if len(names) == 0 {
			return nil
		}

		index := lo.IndexOf(names, gui.currentSideWindowName())
		if index < 0 {
			index = 0
		}

		next := names[((index+offset)%len(names)+len(names))%len(names)]

		view, err := gui.g.View(next)
		if err != nil {
			return err
		}

		return gui.switchFocus(view)
	}
}

func (gui *Gui) allSidePanels() []panels.ISideListPanel {
	return lo.Map(gui.sidePanelDefs(), func(def sidePanelDef, _ int) panels.ISideListPanel {
		return def.panel()
	})
}

func (gui *Gui) allListPanels() []panels.ISideListPanel {
	return append(gui.allSidePanels(), gui.Panels.Menu)
}
