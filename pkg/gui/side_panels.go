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
			name:    "images",
			title:   gui.Tr.ImagesTitle,
			viewPtr: &gui.Views.Images,
			panel:   func() panels.ISideListPanel { return gui.Panels.Images },
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

func (gui *Gui) allSidePanels() []panels.ISideListPanel {
	return lo.Map(gui.sidePanelDefs(), func(def sidePanelDef, _ int) panels.ISideListPanel {
		return def.panel()
	})
}

func (gui *Gui) allListPanels() []panels.ISideListPanel {
	return append(gui.allSidePanels(), gui.Panels.Menu)
}
