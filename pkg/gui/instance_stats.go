package gui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// renderInstanceStatsToMain periodically re-renders CPU/memory/network/disk
// usage from the instance's last-fetched full details (InstanceFull.State),
// which IncusCommand.RefreshInstanceDetails already keeps current in the
// background - no extra API calls needed here, unlike the Logs tab.
func (gui *Gui) renderInstanceStatsToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderStringMain(gui.instanceStatsStr(instance))
		},
		Duration:   time.Second,
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) instanceStatsStr(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok || full.State == nil {
		return gui.Tr.WaitingForInstanceInfo
	}

	state := full.State
	padding := 12
	output := ""

	output += utils.WithPadding("CPU: ", padding) + formatCPUUsage(state.CPU) + "\n"
	output += utils.WithPadding("Memory: ", padding) + formatMemoryUsage(state.Memory) + "\n"
	output += utils.WithPadding("Processes: ", padding) + fmt.Sprint(state.Processes) + "\n"

	output += "\nDisk:\n"
	if len(state.Disk) == 0 {
		output += "  (no disk usage reported)\n"
	} else {
		for _, name := range sortedKeys(state.Disk) {
			output += "  " + utils.WithPadding(name+": ", padding) + formatDiskUsage(state.Disk[name]) + "\n"
		}
	}

	output += "\nNetwork:\n"
	if len(state.Network) == 0 {
		output += "  (no network interfaces reported)\n"
	} else {
		for _, name := range sortedKeys(state.Network) {
			output += "  " + name + ":\n" + formatNetworkInterface(state.Network[name], padding)
		}
	}

	return output
}

// formatNetworkInterface mirrors the per-interface detail `incus info`
// shows: type/state/host-side veth name/MAC/MTU, traffic counters, and any
// assigned IP addresses (with family and scope, e.g. a container's global
// address vs. its link-local one).
func formatNetworkInterface(network api.InstanceStateNetwork, padding int) string {
	output := ""
	output += "    " + utils.WithPadding("Type: ", padding) + network.Type + "\n"
	output += "    " + utils.WithPadding("State: ", padding) + network.State + "\n"
	if network.HostName != "" {
		output += "    " + utils.WithPadding("Host interface: ", padding) + network.HostName + "\n"
	}
	if network.Hwaddr != "" {
		output += "    " + utils.WithPadding("MAC address: ", padding) + network.Hwaddr + "\n"
	}
	output += "    " + utils.WithPadding("MTU: ", padding) + fmt.Sprint(network.Mtu) + "\n"
	output += "    " + utils.WithPadding("Received: ", padding) +
		units.GetByteSizeStringIEC(network.Counters.BytesReceived, 2) +
		fmt.Sprintf(" (%d packets)\n", network.Counters.PacketsReceived)
	output += "    " + utils.WithPadding("Sent: ", padding) +
		units.GetByteSizeStringIEC(network.Counters.BytesSent, 2) +
		fmt.Sprintf(" (%d packets)\n", network.Counters.PacketsSent)

	if len(network.Addresses) > 0 {
		output += "    IP addresses:\n"
		for _, addr := range network.Addresses {
			output += "      " + utils.WithPadding(addr.Family+": ", 7) +
				fmt.Sprintf("%s/%s (%s)\n", addr.Address, addr.Netmask, addr.Scope)
		}
	}

	return output
}

// formatCPUUsage shows total CPU time consumed since the instance started,
// not a percentage: Incus's API reports cumulative usage
// (InstanceStateCPU.Usage, in nanoseconds), not an instantaneous rate, and
// computing a rate would mean tracking deltas between polls ourselves. This
// matches what `incus info <name>` itself shows ("CPU usage (in seconds)").
func formatCPUUsage(cpu api.InstanceStateCPU) string {
	if cpu.Usage <= 0 {
		return "(no usage reported)"
	}
	return fmt.Sprintf("%.2fs total", time.Duration(cpu.Usage).Seconds())
}

func formatMemoryUsage(mem api.InstanceStateMemory) string {
	if mem.Usage <= 0 {
		return "(no usage reported)"
	}

	str := units.GetByteSizeStringIEC(mem.Usage, 2)
	if mem.Total > 0 {
		str += " / " + units.GetByteSizeStringIEC(mem.Total, 2)
	}
	if mem.UsagePeak > 0 {
		str += fmt.Sprintf(" (peak %s)", units.GetByteSizeStringIEC(mem.UsagePeak, 2))
	}
	return str
}

func formatDiskUsage(disk api.InstanceStateDisk) string {
	if disk.Usage <= 0 {
		return "(no usage reported)"
	}

	str := units.GetByteSizeStringIEC(disk.Usage, 2)
	if disk.Total > 0 {
		str += " / " + units.GetByteSizeStringIEC(disk.Total, 2)
	}
	return str
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
