package presentation

import (
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/util"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetInstanceDisplayStrings(guiConfig *config.GuiConfig, instance *commands.Instance) []string {
	return []string{
		getInstanceDisplayStatus(guiConfig, instance),
		instance.Name,
		utils.ColoredString(displayInstanceType(instance), color.FgMagenta),
		utils.ColoredString(displayInstanceAddresses(instance), color.FgYellow),
	}
}

// displayInstanceType mirrors the `incus list` TYPE column: "vm" or
// "container", with an "(app)" suffix for OCI-based application containers
// (`incus` itself checks the volatile.container.oci config key - see
// cmd/incus/list.go's typeColumnData). That flag only appears in the
// instance's expanded config, which we only have once RefreshInstanceDetails
// has fetched full details in the background, so this shows the bare type
// until then.
func displayInstanceType(instance *commands.Instance) string {
	if instance.IsVM() {
		return "vm"
	}

	if full, ok := instance.Full(); ok && util.IsTrue(full.ExpandedConfig["volatile.container.oci"]) {
		return "container (app)"
	}

	return "container"
}

func displayInstanceAddresses(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok || full.State == nil {
		return ""
	}

	addresses := []string{}
	for name, network := range full.State.Network {
		if name == "lo" {
			continue
		}
		for _, addr := range network.Addresses {
			if addr.Scope != "global" {
				continue
			}
			addresses = append(addresses, addr.Address)
		}
	}

	sort.Strings(addresses)

	return strings.Join(addresses, ", ")
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

	return utils.ColoredString(instanceState, getInstanceColor(instance))
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
