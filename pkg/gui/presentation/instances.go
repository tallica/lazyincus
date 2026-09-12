package presentation

import (
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/util"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// instanceColumnRenderers maps each supported InstanceColumns value to the
// function that renders it, so GetInstanceDisplayStrings can show and order
// columns purely based on user config.
var instanceColumnRenderers = map[string]func(*config.GuiConfig, *commands.Instance) string{
	"name":   func(_ *config.GuiConfig, instance *commands.Instance) string { return instance.Name },
	"status": getInstanceDisplayStatus,
	"type": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return utils.ColoredString(displayInstanceType(instance), color.FgMagenta)
	},
	"ipv4": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return utils.ColoredString(displayInstanceAddresses(instance, "inet"), color.FgYellow)
	},
	"ipv6": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return utils.ColoredString(displayInstanceAddresses(instance, "inet6"), color.FgYellow)
	},
	"project": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return utils.ColoredString(instance.Project, color.FgCyan)
	},
	"service": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return utils.ColoredString(instance.ComposeService(), color.FgGreen)
	},
	"snapshots": func(_ *config.GuiConfig, instance *commands.Instance) string {
		return displayInstanceSnapshotCount(instance)
	},
}

func GetInstanceDisplayStrings(guiConfig *config.GuiConfig, instance *commands.Instance, showProject bool) []string {
	columns := guiConfig.InstanceColumns
	if len(columns) == 0 {
		columns = config.DefaultInstanceColumns
	}

	columns = withProjectColumn(columns, showProject)

	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		render, ok := instanceColumnRenderers[column]
		if !ok {
			continue
		}
		cells = append(cells, render(guiConfig, instance))
	}

	return cells
}

// withProjectColumn puts the project first when the list spans projects,
// unless the user already placed it somewhere. Without it the rows of an
// all-projects listing don't say which project they came from, and names
// repeat across projects.
func withProjectColumn(columns []string, spansProjects bool) []string {
	if !spansProjects || lo.Contains(columns, "project") {
		return columns
	}

	return append([]string{"project"}, columns...)
}

// displayInstanceType mirrors the `incus list` TYPE column, including the
// "(app)" suffix for OCI application containers that the CLI derives from
// volatile.container.oci (cmd/incus/list.go, typeColumnData). That key is in
// the expanded config, so the suffix appears only once details are fetched.
func displayInstanceType(instance *commands.Instance) string {
	if instance.IsVM() {
		return "vm"
	}

	if full, ok := instance.Full(); ok && util.IsTrue(full.ExpandedConfig["volatile.container.oci"]) {
		return "container (app)"
	}

	return "container"
}

// displayInstanceAddresses renders the instance's global-scope addresses for
// the given address family the way `incus list` does: space-separated within
// the one column, rather than comma-separated.
func displayInstanceAddresses(instance *commands.Instance, family string) string {
	return strings.Join(instance.Addresses(family), " ")
}

// displayInstanceSnapshotCount mirrors `incus list`'s SNAPSHOTS column.
// Snapshots only appear in InstanceFull, so like the type/address columns
// this shows "0" until RefreshInstanceDetails has fetched full details in
// the background.
func displayInstanceSnapshotCount(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok {
		return "0"
	}

	return strconv.Itoa(len(full.Snapshots))
}

// getInstanceDisplayStatus returns the colored status of the instance
func getInstanceDisplayStatus(guiConfig *config.GuiConfig, instance *commands.Instance) string {
	shortStatusMap := map[string]string{
		"Running":  "R",
		"Stopped":  "X",
		"Frozen":   "P",
		"Error":    "E",
		"Starting": "S",
		"Stopping": "S",
		"Freezing": "P",
		"Thawed":   "T",
	}

	iconStatusMap := map[string]rune{
		"Running":  '▶',
		"Stopped":  '⨯',
		"Frozen":   '◫',
		"Error":    '!',
		"Starting": '⟳',
		"Stopping": '⟳',
		"Freezing": '◫',
		"Thawed":   '▶',
	}

	var instanceState string
	switch guiConfig.InstanceStatusStyle {
	case "short":
		instanceState = shortStatusMap[instance.Instance.Status]
		if instanceState == "" {
			instanceState = instance.Instance.Status
		}
	case "icon":
		if icon, ok := iconStatusMap[instance.Instance.Status]; ok {
			instanceState = string(icon)
		} else {
			instanceState = instance.Instance.Status
		}
	case "long":
		fallthrough
	default:
		instanceState = instance.Instance.Status
	}

	return utils.ColoredString(strings.ToLower(instanceState), getInstanceColor(instance))
}

// getInstanceColor returns the color to use for an instance's status
func getInstanceColor(instance *commands.Instance) color.Attribute {
	switch instance.Instance.Status {
	case "Running":
		return color.FgGreen
	case "Stopped":
		return color.FgRed
	case "Frozen":
		return color.FgYellow
	case "Error":
		return color.FgRed
	case "Starting", "Stopping", "Freezing", "Thawed":
		return color.FgBlue
	default:
		return color.FgWhite
	}
}
