package gui

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/utils"
)

var textColorMap = map[string]color.Attribute{
	"black":   color.FgBlack,
	"red":     color.FgRed,
	"green":   color.FgGreen,
	"yellow":  color.FgYellow,
	"blue":    color.FgBlue,
	"magenta": color.FgMagenta,
	"cyan":    color.FgCyan,
	"white":   color.FgWhite,
}

// colorRemoteFrames gives each panel's border the colour of the remote its
// keys act on; the main panel takes the side panel it's showing.
func (gui *Gui) colorRemoteFrames() {
	for _, panel := range gui.allSidePanels() {
		view := panel.GetView()
		view.FrameColor = gui.remoteFrameColor(gui.actionRemote(view.Name()))
	}

	if gui.Views.Main != nil {
		gui.Views.Main.FrameColor = gui.remoteFrameColor(gui.actionRemote(gui.currentSideViewName()))
	}
}

func (gui *Gui) remoteFrameColor(remote string) gocui.Attribute {
	return GetGocuiAttribute(gui.Config.UserConfig.Remotes[remote].Color)
}

// remoteName is remote in its configured colour, for the footer.
func (gui *Gui) remoteName(remote string) string {
	key := gui.Config.UserConfig.Remotes[remote].Color
	if utils.IsValidHexValue(key) {
		r, g, b := hexRGB(key)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, remote)
	}

	if attribute, ok := textColorMap[key]; ok {
		return utils.ColoredString(remote, attribute)
	}

	return remote
}
