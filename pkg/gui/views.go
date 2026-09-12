package gui

import (
	"os"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/utils"
)

// See https://github.com/xtermjs/xterm.js/issues/4238
var (
	underscoreEnvChecked bool
	hideUnderscores      bool
)

func hideUnderScores() bool {
	if !underscoreEnvChecked {
		hideUnderscores = os.Getenv("TERM_PROGRAM") == "vscode"
		underscoreEnvChecked = true
	}

	return hideUnderscores
}

type Views struct {
	// side panel
	Instances *gocui.View

	// main panel
	Main *gocui.View

	// bottom line
	Options     *gocui.View
	Information *gocui.View
	AppStatus   *gocui.View
	// text that prompts you to enter text in the Filter view
	FilterPrefix *gocui.View
	// appears next to the FilterPrefix view, it's where you type in the search string
	Filter *gocui.View

	// popups
	Confirmation *gocui.View
	Menu         *gocui.View

	// will cover everything when it appears
	Limit *gocui.View
}

type viewNameMapping struct {
	viewPtr **gocui.View
	name    string
	// if true, we handle the position/size of the view in arrangement.go. Otherwise
	// we handle it manually.
	autoPosition bool
}

func (gui *Gui) orderedViewNameMappings() []viewNameMapping {
	return []viewNameMapping{
		{viewPtr: &gui.Views.Instances, name: "instances", autoPosition: true},

		{viewPtr: &gui.Views.Main, name: "main", autoPosition: true},

		// bottom line
		{viewPtr: &gui.Views.Options, name: "options", autoPosition: true},
		{viewPtr: &gui.Views.AppStatus, name: "appStatus", autoPosition: true},
		{viewPtr: &gui.Views.Information, name: "information", autoPosition: true},
		{viewPtr: &gui.Views.Filter, name: "filter", autoPosition: true},
		{viewPtr: &gui.Views.FilterPrefix, name: "filterPrefix", autoPosition: true},

		// popups.
		{viewPtr: &gui.Views.Menu, name: "menu", autoPosition: false},
		{viewPtr: &gui.Views.Confirmation, name: "confirmation", autoPosition: false},

		// this guy will cover everything else when it appears
		{viewPtr: &gui.Views.Limit, name: "limit", autoPosition: true},
	}
}

func (gui *Gui) createAllViews() error {
	frameRunes := []rune{'─', '│', '╭', '╮', '╰', '╯'}
	switch gui.Config.UserConfig.Gui.Border {
	case "single":
		frameRunes = []rune{'─', '│', '┌', '┐', '└', '┘'}
	case "double":
		frameRunes = []rune{'═', '║', '╔', '╗', '╚', '╝'}
	case "hidden":
		frameRunes = []rune{' ', ' ', ' ', ' ', ' ', ' '}
	}

	var err error
	for _, mapping := range gui.orderedViewNameMappings() {
		*mapping.viewPtr, err = gui.prepareView(mapping.name)
		if err != nil && err.Error() != UNKNOWN_VIEW_ERROR_MSG {
			return err
		}
		(*mapping.viewPtr).FrameRunes = frameRunes
		(*mapping.viewPtr).FgColor = gocui.ColorDefault
	}

	selectedLineBgColor := GetGocuiStyle(gui.Config.UserConfig.Gui.Theme.SelectedLineBgColor)

	gui.Views.Main.Wrap = gui.Config.UserConfig.Gui.WrapMainPanel
	gui.Views.Main.IgnoreCarriageReturns = true

	gui.Views.Instances.Highlight = true
	gui.Views.Instances.SelBgColor = selectedLineBgColor
	gui.Views.Instances.Title = gui.Tr.InstancesTitle
	gui.Views.Instances.TitlePrefix = "[1]"

	gui.Views.Options.Frame = false
	gui.Views.Options.FgColor = gui.GetOptionsPanelTextColor()

	gui.Views.AppStatus.FgColor = gocui.ColorCyan
	gui.Views.AppStatus.Frame = false

	gui.Views.Information.Frame = false
	gui.Views.Information.FgColor = gocui.ColorGreen

	gui.Views.Confirmation.Visible = false
	gui.Views.Confirmation.Wrap = true
	gui.Views.Menu.Visible = false
	gui.Views.Menu.SelBgColor = selectedLineBgColor

	gui.Views.Limit.Visible = false
	gui.Views.Limit.Title = gui.Tr.NotEnoughSpace
	gui.Views.Limit.Wrap = true

	gui.Views.FilterPrefix.BgColor = gocui.ColorDefault
	gui.Views.FilterPrefix.FgColor = gocui.ColorGreen
	gui.Views.FilterPrefix.Frame = false

	gui.Views.Filter.BgColor = gocui.ColorDefault
	gui.Views.Filter.FgColor = gocui.ColorGreen
	gui.Views.Filter.Editable = true
	gui.Views.Filter.Frame = false
	gui.Views.Filter.Editor = gocui.EditorFunc(gui.wrapEditor(gocui.SimpleEditor))

	return nil
}

func (gui *Gui) setInitialViewContent() error {
	if err := gui.renderString(gui.g, "information", gui.getInformationContent()); err != nil {
		return err
	}

	_ = gui.setViewContent(gui.Views.FilterPrefix, gui.filterPrompt())

	return nil
}

func (gui *Gui) getInformationContent() string {
	informationStr := gui.incusStatusContent() + gui.Config.Version
	if !gui.g.Mouse {
		return informationStr
	}

	attrs := []color.Attribute{color.FgMagenta}
	if !hideUnderScores() {
		attrs = append(attrs, color.Underline)
	}

	donate := color.New(attrs...).Sprint(gui.Tr.Donate)
	return donate + " " + informationStr
}

// incusStatusContent renders the server version, the remote and project the
// instance list is scoped to, and a connection indicator - e.g.
// "Incus v6.5 (colima/default) ● ". Empty until we have a server version.
func (gui *Gui) incusStatusContent() string {
	remote := gui.IncusCommand.RemoteName
	version := gui.IncusCommand.ServerVersion
	if remote == "" && version == "" {
		return ""
	}

	label := "Incus"
	if version != "" {
		label += " v" + version
	}
	scope := remote
	if project := gui.IncusCommand.ProjectName(); project != "" {
		if scope == "" {
			scope = project
		} else {
			scope += "/" + project
		}
	}
	if scope != "" {
		label += " (" + scope + ")"
	}

	if gui.IncusCommand.IsConnected() {
		label += " " + utils.ColoredString("●", color.FgGreen)
	} else {
		label += " " + utils.ColoredString("✗", color.FgRed)
	}

	return label + "  "
}

func (gui *Gui) popupViewNames() []string {
	return []string{"confirmation", "menu"}
}

// these views have their position and size determined by arrangement.go
func (gui *Gui) autoPositionedViewNames() []string {
	views := lo.Filter(gui.orderedViewNameMappings(), func(viewNameMapping viewNameMapping, _ int) bool {
		return viewNameMapping.autoPosition
	})

	return lo.Map(views, func(viewNameMapping viewNameMapping, _ int) string {
		return viewNameMapping.name
	})
}
