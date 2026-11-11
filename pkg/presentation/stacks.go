package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// stackErrorStyles spells a stack whose compose config couldn't be read.
var stackErrorStyles = map[string]string{"short": "!", "icon": "✗"}

// stackUnreachableStyles spells a stack whose remote didn't answer, and
// stackConnectingStyles one whose remote hasn't yet.
var (
	stackUnreachableStyles = map[string]string{"short": "?", "icon": "?"}
	stackConnectingStyles  = map[string]string{"short": "…", "icon": "…"}
)

// StackRemote is a Stacks row's remote column, for a list with a stack
// elsewhere: the stack's remote, marked the way the remotes menu marks it
// when it's the session's.
type StackRemote struct {
	Name   string
	Active bool
}

// GetStackDisplayStrings is a Stacks row: its remote when there's one to
// show, then name, status and directory, home written as `~`.
func GetStackDisplayStrings(
	guiConfig *config.GuiConfig, stack *commands.ComposeStack, home string, remote *StackRemote,
	updates map[string]commands.UpstreamStatus,
) []string {
	cells := []string{
		stack.Title(),
		withStackMarkers(displayStackStatus(guiConfig, stack), stack, updates),
		commands.ShortenHome(stack.Dir, home),
	}

	if remote == nil {
		return cells
	}

	name := "  " + remote.Name
	if remote.Active {
		name = utils.ColoredString("* "+remote.Name, color.FgGreen)
	}

	return append([]string{name}, cells...)
}

// withStackMarkers adds ↑ for a newer build or tag, else ! for beacon's error.
func withStackMarkers(status string, stack *commands.ComposeStack, updates map[string]commands.UpstreamStatus) string {
	switch {
	case imageNewer(updates, stack.Images...) || stack.Beacon.Status == commands.BeaconUpdateAvailable:
		return status + " " + newerMarker()
	case stack.Beacon.Status == commands.BeaconError:
		return status + " " + utils.ColoredString("!", color.FgRed)
	}

	return status
}

// StackPath is the stack's Ref with home written as `~`.
func StackPath(stack *commands.ComposeStack, home string) string {
	return commands.StackRef(stack.Remote, commands.ShortenHome(stack.Dir, home))
}

func displayStackStatus(guiConfig *config.GuiConfig, stack *commands.ComposeStack) string {
	switch {
	case stack.Err != nil:
		return styledStackState(guiConfig, "error", stackErrorStyles, color.FgRed)
	case stack.StatusErr != nil:
		return styledStackState(guiConfig, "unreachable", stackUnreachableStyles, color.FgRed)
	case stack.StatusPending:
		return styledStackState(guiConfig, "connecting", stackConnectingStyles, color.FgYellow)
	case len(stack.Changed()) > 0:
		return DisplayRolledUpStatus(guiConfig, stack.Status()) + " " + displayChanged(guiConfig, true)
	default:
		return DisplayRolledUpStatus(guiConfig, stack.Status())
	}
}

func styledStackState(guiConfig *config.GuiConfig, display string, styles map[string]string, colour color.Attribute) string {
	if styled, ok := styles[guiConfig.InstanceStatusStyle]; ok {
		display = styled
	}

	return utils.ColoredString(display, colour)
}
