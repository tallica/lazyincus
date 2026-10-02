package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// stackErrorStyles spells a stack whose compose config couldn't be read.
var stackErrorStyles = map[string]string{"short": "!", "icon": "✗"}

// stackUnreachableStyles spells a stack whose remote didn't answer.
var stackUnreachableStyles = map[string]string{"short": "?", "icon": "?"}

// StackRemote is a Stacks row's remote column, for a list with a stack
// elsewhere: the stack's remote, coloured when it isn't the session's.
type StackRemote struct {
	Name      string
	Elsewhere bool
}

// GetStackDisplayStrings is a Stacks row: name, status and directory, the
// directory under home written with `~`, after the remote when there's one
// to show.
func GetStackDisplayStrings(guiConfig *config.GuiConfig, stack *commands.ComposeStack, home string, remote *StackRemote) []string {
	cells := []string{
		stack.Title(),
		displayStackStatus(guiConfig, stack),
		commands.ShortenHome(stack.Dir, home),
	}

	if remote == nil {
		return cells
	}

	name := remote.Name
	if remote.Elsewhere {
		name = utils.ColoredString(name, color.FgMagenta)
	}

	return append([]string{name}, cells...)
}

// StackPath is the stack's Ref with home written as `~`.
func StackPath(stack *commands.ComposeStack, home string) string {
	return commands.StackRef(stack.Remote, commands.ShortenHome(stack.Dir, home))
}

func displayStackStatus(guiConfig *config.GuiConfig, stack *commands.ComposeStack) string {
	switch {
	case stack.Err != nil:
		return styledStackFailure(guiConfig, "error", stackErrorStyles)
	case stack.StatusErr != nil:
		return styledStackFailure(guiConfig, "unreachable", stackUnreachableStyles)
	default:
		return DisplayRolledUpStatus(guiConfig, stack.Status())
	}
}

func styledStackFailure(guiConfig *config.GuiConfig, display string, styles map[string]string) string {
	if styled, ok := styles[guiConfig.InstanceStatusStyle]; ok {
		display = styled
	}

	return utils.ColoredString(display, color.FgRed)
}
