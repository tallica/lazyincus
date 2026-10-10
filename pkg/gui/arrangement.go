package gui

import (
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/utils"
)

// In this file we use the boxlayout package, along with knowledge about the app's state,
// to arrange the windows (i.e. panels) on the screen.

const INFO_SECTION_PADDING = " "

func (gui *Gui) getWindowDimensions(informationStr string, appStatus string) map[string]boxlayout.Dimensions {
	minimumHeight := 9
	minimumWidth := 10
	width, height := gui.g.Size()
	if width < minimumWidth || height < minimumHeight {
		return boxlayout.ArrangeWindows(&boxlayout.Box{Window: "limit"}, 0, 0, width, height)
	}

	sideSectionWeight, mainSectionWeight := gui.getMidSectionWeights()

	sidePanelsDirection := boxlayout.COLUMN
	portraitMode := width <= 84 && height > 45
	if portraitMode {
		sidePanelsDirection = boxlayout.ROW
	}

	showInfoSection := gui.Config.UserConfig.Gui.ShowBottomLine || gui.State.Filter.active
	infoSectionSize := 0
	if showInfoSection {
		infoSectionSize = 1
	}

	root := &boxlayout.Box{
		Direction: boxlayout.ROW,
		Children: []*boxlayout.Box{
			{
				Direction: sidePanelsDirection,
				Weight:    1,
				Children: []*boxlayout.Box{
					{
						Direction:           boxlayout.ROW,
						Weight:              sideSectionWeight,
						ConditionalChildren: gui.sidePanelChildren,
					},
					{
						Window: "main",
						Weight: mainSectionWeight,
					},
				},
			},
			{
				Direction: boxlayout.COLUMN,
				Size:      infoSectionSize,
				Children:  gui.infoSectionChildren(informationStr, appStatus),
			},
		},
	}

	return boxlayout.ArrangeWindows(root, 0, 0, width, height)
}

func (gui *Gui) getMidSectionWeights() (int, int) {
	currentWindow := gui.currentStaticWindowName()

	sidePanelWidthRatio := gui.Config.UserConfig.Gui.SidePanelWidth
	mainSectionWeight := int(1/sidePanelWidthRatio) - 1
	sideSectionWeight := 1

	if currentWindow == "main" && gui.State.ScreenMode == SCREEN_FULL {
		mainSectionWeight = 1
		sideSectionWeight = 0
	} else {
		switch gui.State.ScreenMode {
		case SCREEN_NORMAL:
			// keep the configured side-panel ratio
		case SCREEN_HALF:
			mainSectionWeight = 1
		case SCREEN_FULL:
			mainSectionWeight = 0
		}
	}

	return sideSectionWeight, mainSectionWeight
}

func (gui *Gui) infoSectionChildren(informationStr string, appStatus string) []*boxlayout.Box {
	result := []*boxlayout.Box{}

	if len(appStatus) > 0 {
		result = append(result,
			&boxlayout.Box{
				Window: "appStatus",
				Size:   utils.DisplayWidth(appStatus) + utils.DisplayWidth(INFO_SECTION_PADDING),
			},
		)
	}

	if gui.State.Filter.active {
		return append(result, []*boxlayout.Box{
			{
				Window: "filterPrefix",
				Size:   utils.DisplayWidth(gui.filterPrompt()),
			},
			{
				Window: "filter",
				Weight: 1,
			},
		}...)
	}

	result = append(result,
		[]*boxlayout.Box{
			{
				Window: "options",
				Weight: 1,
			},
			{
				Window: "information",
				Size:   utils.DisplayWidth(informationStr),
			},
		}...,
	)

	return result
}

func (gui *Gui) sideViewNames() []string {
	return lo.Map(gui.visibleSidePanelDefs(), func(def sidePanelDef, _ int) string {
		return def.name
	})
}

// collapsedSidePanelHeight is a title bar plus one row of content, so a
// collapsed panel still shows what it is and whether it has anything in it.
const collapsedSidePanelHeight = 3

func (gui *Gui) sidePanelChildren(width int, height int) []*boxlayout.Box {
	sideWindowNames := gui.sideWindowNames()

	if gui.State.ExpandSidePanel {
		return gui.expandedSidePanelChildren(sideWindowNames, height)
	}

	focused := gui.currentSideWindowName()
	fits := gui.collapsedFit(sideWindowNames, height)

	// Equal weights: the side section splits evenly between however many
	// panels are visible, but for stacks that have nothing on this remote.
	return lo.Map(sideWindowNames, func(window string, _ int) *boxlayout.Box {
		if fits && gui.stacksCollapsed(window, focused) {
			return &boxlayout.Box{Window: window, Size: titleOnlyHeight}
		}

		return &boxlayout.Box{
			Window: window,
			Weight: 1,
		}
	})
}

// titleOnlyHeight is a frame with no rows inside, its title all it shows.
const titleOnlyHeight = 2

// stacksCollapsed is whether window shrinks to its title on a remote with
// no stack listed: Stacks and Services, until either is focused, and then
// Instances instead.
func (gui *Gui) stacksCollapsed(window, focused string) bool {
	if !gui.stacksAway() {
		return false
	}

	stacksFocused := focused == "stacks" || focused == "services"
	if window == "stacks" || window == "services" {
		return !stacksFocused
	}

	return window == "instances" && stacksFocused
}

// stacksAway is whether Stacks and Services give way to Instances: no stack
// is on the session's remote, and the config hasn't turned that off.
func (gui *Gui) stacksAway() bool {
	return !gui.State.StacksHere && gui.Config.UserConfig.Gui.CollapseStacksElsewhere
}

// collapsedFit is whether every panel but one could be collapsed and still
// leave that one room - better a cramped list than panels squeezed out of
// existence.
func (gui *Gui) collapsedFit(sideWindowNames []string, height int) bool {
	return height >= len(sideWindowNames)*collapsedSidePanelHeight+collapsedSidePanelHeight
}

// expandedSidePanelChildren gives the focused panel everything the collapsed
// ones don't need. The focused panel is the last side panel that had focus,
// so moving into the main panel doesn't collapse the list you were reading.
// Falls back to an even split when the panels can't all fit collapsed.
func (gui *Gui) expandedSidePanelChildren(sideWindowNames []string, height int) []*boxlayout.Box {
	focused := gui.currentSideWindowName()

	if !gui.collapsedFit(sideWindowNames, height) {
		return lo.Map(sideWindowNames, func(window string, _ int) *boxlayout.Box {
			return &boxlayout.Box{Window: window, Weight: 1}
		})
	}

	return lo.Map(sideWindowNames, func(window string, _ int) *boxlayout.Box {
		if window == focused {
			return &boxlayout.Box{Window: window, Weight: 1}
		}

		return &boxlayout.Box{Window: window, Size: collapsedSidePanelHeight}
	})
}
