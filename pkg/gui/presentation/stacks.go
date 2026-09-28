package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// stackErrorStyles spells a stack whose compose config couldn't be read.
var stackErrorStyles = map[string]string{"short": "!", "icon": "✗"}

// GetStackDisplayStrings is a Stacks row: name, status and directory, the
// directory under home written with `~`.
func GetStackDisplayStrings(guiConfig *config.GuiConfig, stack *commands.ComposeStack, home string) []string {
	return []string{
		stack.Title(),
		displayStackStatus(guiConfig, stack),
		commands.ShortenHome(stack.Dir, home),
	}
}

func displayStackStatus(guiConfig *config.GuiConfig, stack *commands.ComposeStack) string {
	if stack.Err == nil {
		return DisplayRolledUpStatus(guiConfig, stack.Status())
	}

	display := "error"
	if styled, ok := stackErrorStyles[guiConfig.InstanceStatusStyle]; ok {
		display = styled
	}

	return utils.ColoredString(display, color.FgRed)
}
