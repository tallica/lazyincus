package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
)

type screenMode int

const (
	screenNormal screenMode = iota
	screenHalf
	screenFull
)

func parseScreenMode(mode string) screenMode {
	switch mode {
	case "half":
		return screenHalf
	case "full", "fullscreen":
		return screenFull
	}

	return screenNormal
}

// nextScreenMode is `+` (1) and `_` (-1), reversed while main has focus.
func (m *Model) nextScreenMode(step int) {
	if m.currentView() == mainView {
		step = -step
	}

	m.screenMode = screenMode(wrap(int(m.screenMode)+step, 3))
}

const (
	mainView        = "main"
	limitView       = "limit"
	optionsView     = "options"
	informationView = "information"
	appStatusView   = "appStatus"
)

// infoSectionPadding keeps cut-off options a space clear of the information.
const infoSectionPadding = " "

func (m *Model) dimensions(width, height int, information, appStatus string) map[string]boxlayout.Dimensions {
	if width < 10 || height < 9 {
		return boxlayout.ArrangeWindows(&boxlayout.Box{Window: limitView}, 0, 0, width, height)
	}

	sideWeight, mainWeight := m.midSectionWeights()

	direction := boxlayout.COLUMN
	if width <= 84 && height > 45 {
		direction = boxlayout.ROW
	}

	infoSize := 0
	if m.cfg.UserConfig.Gui.ShowBottomLine || m.filter.active {
		infoSize = 1
	}

	root := &boxlayout.Box{
		Direction: boxlayout.ROW,
		Children: []*boxlayout.Box{
			{
				Direction: direction,
				Weight:    1,
				Children: []*boxlayout.Box{
					{Direction: boxlayout.ROW, Weight: sideWeight, ConditionalChildren: m.sideChildren},
					{Window: mainView, Weight: mainWeight},
				},
			},
			{Direction: boxlayout.COLUMN, Size: infoSize, Children: m.infoChildren(information, appStatus)},
		},
	}

	return boxlayout.ArrangeWindows(root, 0, 0, width, height)
}

func (m *Model) midSectionWeights() (int, int) {
	sideWeight := 1
	mainWeight := int(1/m.cfg.UserConfig.Gui.SidePanelWidth) - 1

	if m.currentStaticView() == mainView && m.screenMode == screenFull {
		return 0, 1
	}

	switch m.screenMode {
	case screenHalf:
		mainWeight = 1
	case screenFull:
		mainWeight = 0
	case screenNormal:
	}

	return sideWeight, mainWeight
}

func (m *Model) infoChildren(information, appStatus string) []*boxlayout.Box {
	children := []*boxlayout.Box{}
	if appStatus != "" {
		children = append(children, &boxlayout.Box{
			Window: appStatusView,
			Size:   ansi.StringWidth(appStatus) + ansi.StringWidth(infoSectionPadding),
		})
	}

	if m.filter.active {
		return append(children,
			&boxlayout.Box{Window: filterPrefixView, Size: ansi.StringWidth(m.filterPrompt())},
			&boxlayout.Box{Window: filterView, Weight: 1},
		)
	}

	return append(children,
		&boxlayout.Box{Window: optionsView, Weight: 1},
		&boxlayout.Box{Window: informationView, Size: ansi.StringWidth(information)},
	)
}

// collapsedHeight keeps one row under the title; titleOnlyHeight none.
const (
	collapsedHeight = 3
	titleOnlyHeight = 2
)

func (m *Model) sideChildren(_, height int) []*boxlayout.Box {
	windows := m.windowNames()
	focused := m.windowOf(m.currentSideView())
	fits := height >= len(windows)*collapsedHeight+collapsedHeight

	children := make([]*boxlayout.Box, len(windows))
	for i, window := range windows {
		children[i] = &boxlayout.Box{Window: window, Weight: 1}

		switch {
		case m.expandSidePanel && fits && window != focused:
			children[i] = &boxlayout.Box{Window: window, Size: collapsedHeight}
		case !m.expandSidePanel && fits && m.stacksCollapsed(window, focused):
			children[i] = &boxlayout.Box{Window: window, Size: titleOnlyHeight}
		}
	}

	return children
}

func (m *Model) stacksCollapsed(window, focused string) bool {
	if !m.stacksAway() {
		return false
	}

	stacksFocused := focused == "stacks" || focused == "services"
	if window == "stacks" || window == "services" {
		return !stacksFocused
	}

	return window == "instances" && stacksFocused
}
