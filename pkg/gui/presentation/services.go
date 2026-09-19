package presentation

import (
	"strconv"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetComposeServiceDisplayStrings(service *commands.ComposeService) []string {
	return []string{
		displayServiceStatus(service),
		service.Name,
		displayServiceReplicas(service),
		utils.ColoredString(utils.Truncate(shortImageRef(service.Image), maxImageAliasWidth), color.FgBlue),
	}
}

// displayServiceReplicas is running-against-declared, the one number pair
// that says whether the stack is where the compose file asked it to be.
func displayServiceReplicas(service *commands.ComposeService) string {
	running := strconv.Itoa(len(service.Instances))

	if service.Replicas == len(service.Instances) {
		return running
	}

	return running + "/" + strconv.Itoa(service.Replicas)
}

// "none" is white rather than red: a service the compose file declares and
// nothing is running is the normal state of a stack that's down, not a fault.
func displayServiceStatus(service *commands.ComposeService) string {
	status := service.Status()

	var statusColor color.Attribute

	switch status {
	case commands.ServiceRunning:
		statusColor = color.FgGreen
	case commands.ServicePartial:
		statusColor = color.FgYellow
	case commands.ServiceStopped:
		statusColor = color.FgRed
	default:
		statusColor = color.FgWhite
	}

	return utils.ColoredString(status, statusColor)
}
