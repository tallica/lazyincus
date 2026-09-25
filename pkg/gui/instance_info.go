package gui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/gui/presentation"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// renderInstanceInfoToMain periodically re-renders what the instance is and
// what it's doing: identity, then the Stats section underneath, both from
// the newest instance listing - no API calls of its own, unlike the Logs
// tab.
func (gui *Gui) renderInstanceInfoToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.instanceInfoStr(instance.Latest()))
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

// instanceInfoStr takes the labels of any identity lines to leave out: the
// services panel stacks this under a service that has already said those.
// The counters underneath are set off by a gap rather than a heading of
// their own: CPU and memory need no label to say what they are, and a
// ruled heading here would carry the same weight as the replica headings
// stacking whole instances, which are a level above it.
func (gui *Gui) instanceInfoStr(instance *commands.Instance, omit ...string) string {
	return gui.instanceIdentityStr(instance, omit...) + "\n\n" + gui.instanceStatsStr(instance)
}

// sectionHeading marks off the blocks a tab is made of, the tab being
// several things stacked rather than one table. The label sits inside the
// rule the way a fieldset legend sits in its border: a service's replicas
// stack the same labels several times over, and a heading with nothing
// under it doesn't stop the eye.
func (gui *Gui) sectionHeading(title string) string {
	rule := "\u2500\u2500 " + title + " "

	if padding := int(gui.mainViewWidth.Load()) - utils.DisplayWidth(rule); padding > 0 {
		rule += strings.Repeat("\u2500", padding)
	} else {
		rule += "\u2500\u2500\u2500"
	}

	return utils.ColoredString(rule, color.FgCyan)
}

// instanceIdentityStr is what `incus info` prints before the counters,
// minus what's a tab of its own: no config, no profiles list, no snapshot
// dates. Fields an instance may not have - a compose image, a health
// verdict, addresses - are left out rather than shown empty.
// identityPadding lines every label's value up in the same column, the Info
// tab being one block of them once the identity lines and the counters run
// together.
const identityPadding = 14

func (gui *Gui) instanceIdentityStr(instance *commands.Instance, omit ...string) string {
	padding := identityPadding

	line := func(label, value string) string {
		if value == "" || lo.Contains(omit, label) {
			return ""
		}

		return utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := line("Name", instance.Name)
	output += line("Status", strings.ToLower(instance.Instance.Status))
	output += line("Type", presentation.InstanceType(instance))
	output += line("Project", instance.Project)
	output += line("Image", instance.Image())
	output += line("Health", instance.HealthStatus())
	output += line("Architecture", instance.Instance.Architecture)
	output += line("Created", localTime(instance.Instance.CreatedAt))
	output += line("Last used", localTime(instance.Instance.LastUsedAt))
	output += line("IPv4", strings.Join(instance.Addresses("inet"), " "))
	output += line("IPv6", strings.Join(instance.Addresses("inet6"), " "))

	output += line("Snapshots", strconv.Itoa(len(instance.Instance.Snapshots)))

	return output
}

// localTime is blank for the zero time an instance that has never run
// reports as its last use.
func localTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.Local().Format(presentation.DateTimeFormat)
}

func (gui *Gui) instanceStatsStr(instance *commands.Instance) string {
	state := instance.Instance.State
	if state == nil {
		return ""
	}

	// The nested labels under Disk and Network keep a padding of their own,
	// being a level in.
	padding := 12
	output := ""

	output += utils.WithPadding("CPU: ", identityPadding) + formatCPUUsage(state.CPU) + "\n"
	output += utils.WithPadding("Memory: ", identityPadding) + formatMemoryUsage(state.Memory) + "\n"
	output += utils.WithPadding("Processes: ", identityPadding) + fmt.Sprint(state.Processes) + "\n"

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

// formatCPUUsage shows cumulative CPU time, not a percentage: the API
// reports total nanoseconds since start, and `incus info` shows the same.
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
