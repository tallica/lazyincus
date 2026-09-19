package presentation

import (
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetComposeProjectDisplayStrings(project *commands.ComposeProject) []string {
	return []string{
		project.Name,
		displayComposeLocal(project),
	}
}

// displayComposeLocal marks the one project `u`/`d` can act on - the compose
// file in lazyincus's own working directory, not just any project
// incus-compose happens to manage on the server.
func displayComposeLocal(project *commands.ComposeProject) string {
	if project.Local {
		return utils.ColoredString("local", color.FgGreen)
	}

	return ""
}
