package presentation

import (
	"strconv"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

func GetNetworkDisplayStrings(network *commands.Network) []string {
	return []string{
		network.Name,
		utils.ColoredString(network.Network.Type, color.FgMagenta),
		displayNetworkManaged(network),
		utils.ColoredString(strconv.Itoa(network.UsedByCount()), color.FgYellow),
	}
}

// displayNetworkManaged marks the networks Incus controls; the rest are
// host interfaces it merely knows about, and can't be edited or deleted.
func displayNetworkManaged(network *commands.Network) string {
	if network.IsManaged() {
		return utils.ColoredString("managed", color.FgGreen)
	}

	return utils.ColoredString("unmanaged", color.FgBlue)
}
