package presentation

import (
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
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
		return DisplayRolledUpStatus(guiConfig, service.Status())
	},
	"replicas": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return ServiceReplicas(service)
	},
	"health": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return displayHealth(service.Health())
	},
	"image": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		return shortImageRef(service.ResolvedImage())
	},
	"type": func(_ *config.GuiConfig, service *commands.ComposeService) string {
		if len(service.Instances) == 0 {
			return ""
		}

		return utils.ColoredString(InstanceType(service.SortedInstances()[0]), color.FgMagenta)
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

// ServiceImageColumn is InstanceImageColumn for the services panel.
func ServiceImageColumn(guiConfig *config.GuiConfig) int {
	return lo.IndexOf(serviceColumns(guiConfig), "image")
}

// serviceColumns are the columns shown, in order, unknown names dropped.
func serviceColumns(guiConfig *config.GuiConfig) []string {
	columns := guiConfig.ServiceColumns
	if len(columns) == 0 {
		columns = config.DefaultServiceColumns
	}

	return lo.Filter(columns, func(column string, _ int) bool {
		_, ok := serviceColumnRenderers[column]
		return ok
	})
}

// replicaIndent sets a replica's row under the service it belongs to, the
// rows being one list rather than two panels.
const replicaIndent = "  "

func GetServiceRowDisplayStrings(guiConfig *config.GuiConfig, row *commands.ServiceRow) []string {
	columns := serviceColumns(guiConfig)

	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		render := serviceColumnRenderers[column]

		if row.Instance == nil {
			cells = append(cells, render(guiConfig, row.Service))
			continue
		}

		cells = append(cells, replicaCell(guiConfig, column, row.Instance))
	}

	return cells
}

// replicaCell renders a replica's row through the instances panel's own
// renderers - the same column name means the same thing on an instance -
// leaving a cell blank where only a service has the column, so the two
// kinds of row stay in the same table.
func replicaCell(guiConfig *config.GuiConfig, column string, instance *commands.Instance) string {
	if column == "name" {
		return replicaIndent + instance.Name
	}

	render, ok := instanceColumnRenderers[column]
	if !ok {
		return ""
	}

	return render(guiConfig, instance)
}

// displayServiceSnapshotCount totals the replicas': a service's snapshots
// are its instances', however many instances that is.
func displayServiceSnapshotCount(service *commands.ComposeService) string {
	count := 0

	for _, instance := range service.Instances {
		count += len(instance.Instance.Snapshots)
	}

	return strconv.Itoa(count)
}

// displayServiceAddresses is the service's instance's addresses, the way a
// single instance's several already run together in the one column. A
// service with replicas leaves the column to them: each has a row of its
// own carrying its own address, and four sets of those side by side push
// the columns after them off the panel.
func displayServiceAddresses(service *commands.ComposeService, family string) string {
	if len(service.Instances) > 1 {
		return ""
	}

	addresses := make([]string, 0, len(service.Instances))

	for _, instance := range service.Instances {
		addresses = append(addresses, instance.Addresses(family)...)
	}

	return strings.Join(addresses, " ")
}

// ServiceReplicas is how many instances the service has against how many
// the compose file declared, and blank when the two agree: a count that
// always reads "1" is a column of noise. It counts what exists rather than
// what's running, the status column and the replica rows already saying
// what state they're in, so the figure before the slash is the number of
// rows underneath and "3/4" is one replica missing, never one stopped.
func ServiceReplicas(service *commands.ComposeService) string {
	if len(service.Instances) == service.Replicas {
		return ""
	}

	return strconv.Itoa(len(service.Instances)) + "/" + strconv.Itoa(service.Replicas)
}

// serviceStatusStyles covers the two states no instance is ever in; the
// short and icon spellings have nothing to borrow for them.
var serviceStatusStyles = map[string]map[string]string{
	"short": {commands.ServicePartial: "P", commands.ServiceNone: "-"},
	"icon":  {commands.ServicePartial: "◐", commands.ServiceNone: "·"},
}

// DisplayRolledUpStatus hands an instance status to the instances panel's
// own renderer, a service or a stack being its instances, and styles the
// two states that are a rollup's alone. "none" is white rather than red:
// declared and not running is the normal state of a stack that's down, not
// a fault.
func DisplayRolledUpStatus(guiConfig *config.GuiConfig, status string) string {
	statusColor := color.FgWhite

	switch status {
	case commands.ServicePartial:
		statusColor = color.FgYellow
	case commands.ServiceNone:
	default:
		return DisplayStatus(guiConfig, status)
	}

	display := status
	if styled, ok := serviceStatusStyles[guiConfig.InstanceStatusStyle][status]; ok {
		display = styled
	}

	return utils.ColoredString(display, statusColor)
}
