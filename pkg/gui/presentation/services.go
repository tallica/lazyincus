package presentation

import (
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// serviceColumnRenderers is the Services panel's counterpart to
// instanceColumnRenderers: the same column names, rendered from the
// service's instances rolled up, plus "replicas" which only a service has.
var serviceColumnRenderers = map[string]func(*config.GuiConfig, *commands.ComposeService) string{
	"name": func(_ *config.GuiConfig, service *commands.ComposeService) string { return service.Name },
	"status": func(guiConfig *config.GuiConfig, service *commands.ComposeService) string {
		return displayServiceStatus(guiConfig, service)
	},
	"replicas": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return ServiceReplicas(service)
	},
	"health": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return displayHealth(service.Health())
	},
	"image": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		image := shortImageRef(service.ResolvedImage())
		return utils.ColoredString(utils.Truncate(image, maxImageAliasWidth), color.FgBlue)
	},
	"type": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		if len(service.Instances) == 0 {
			return ""
		}

		return utils.ColoredString(InstanceType(service.Instances[0]), color.FgMagenta)
	},
	"snapshots": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return displayServiceSnapshotCount(service)
	},
	"ipv4": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return utils.ColoredString(displayServiceAddresses(service, "inet"), color.FgYellow)
	},
	"ipv6": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return utils.ColoredString(displayServiceAddresses(service, "inet6"), color.FgYellow)
	},
}

func GetComposeServiceDisplayStrings(guiConfig *config.GuiConfig, service *commands.ComposeService) []string {
	columns := guiConfig.ServiceColumns
	if len(columns) == 0 {
		columns = config.DefaultServiceColumns
	}

	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		render, ok := serviceColumnRenderers[column]
		if !ok {
			continue
		}
		cells = append(cells, render(guiConfig, service))
	}

	return cells
}

// displayServiceSnapshotCount totals the replicas': a service's snapshots
// are its instances', however many instances that is.
func displayServiceSnapshotCount(service *commands.ComposeService) string {
	count := 0

	for _, instance := range service.Instances {
		if full, ok := instance.Full(); ok {
			count += len(full.Snapshots)
		}
	}

	return strconv.Itoa(count)
}

// displayServiceAddresses runs the replicas' addresses together in the one
// column, the way a single instance's several addresses already are.
func displayServiceAddresses(service *commands.ComposeService, family string) string {
	addresses := make([]string, 0, len(service.Instances))

	for _, instance := range service.Instances {
		addresses = append(addresses, instance.Addresses(family)...)
	}

	return strings.Join(addresses, " ")
}

// ServiceReplicas is running-against-declared, and blank when the
// two agree: a count that always reads "1" is a column of noise, and the
// Info tab drops its line on the same rule.
func ServiceReplicas(service *commands.ComposeService) string {
	if service.Replicas == len(service.Instances) {
		return ""
	}

	return strconv.Itoa(len(service.Instances)) + "/" + strconv.Itoa(service.Replicas)
}

// serviceStatusStyles renders a rolled-up service state the three ways
// gui.instanceStatusStyle renders an instance's, so one setting covers both
// panels. Partial and none are the service's own: no instance is ever in
// either state, so the instance map has nothing to borrow for them.
var serviceStatusStyles = map[string]map[string]string{
	"short": {
		commands.ServiceRunning: "R",
		commands.ServiceStopped: "X",
		commands.ServicePartial: "P",
		commands.ServiceNone:    "-",
	},
	"icon": {
		commands.ServiceRunning: "▶",
		commands.ServiceStopped: "⨯",
		commands.ServicePartial: "◐",
		commands.ServiceNone:    "·",
	},
}

// "none" is white rather than red: a service the compose file declares and
// nothing is running is the normal state of a stack that's down, not a fault.
func displayServiceStatus(guiConfig *config.GuiConfig, service *commands.ComposeService) string {
	status := service.Status()

	display := status
	if styled, ok := serviceStatusStyles[guiConfig.InstanceStatusStyle][status]; ok {
		display = styled
	}

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

	return utils.ColoredString(display, statusColor)
}
