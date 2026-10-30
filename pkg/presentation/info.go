package presentation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// IdentityPadding lines every label's value up in the same column, the Info
// tab being one block of them once the identity lines and the counters run
// together.
const IdentityPadding = 14

// Location is where an item lives, broadest first, for the lines that head
// its Info or Config tab: with several remotes, projects and stacks about,
// the list it came from no longer says.
type Location struct {
	Remote  string
	Project string
	// Stack and Service are a compose instance's, empty for anything else:
	// the stack is where it's listed from, its name being the project's.
	Stack   string
	Service string
}

// LocationLines is loc's lines, those it has no value for left out, and
// those omit names too: a tab whose heading has already said them.
func LocationLines(loc Location, omit ...string) string {
	output := ""

	for _, line := range [][2]string{
		{"Remote", loc.Remote},
		{"Project", loc.Project},
		{"Stack", loc.Stack},
		{"Service", loc.Service},
	} {
		if line[1] != "" && !lo.Contains(omit, line[0]) {
			output += utils.WithPadding(line[0]+": ", IdentityPadding) + line[1] + "\n"
		}
	}

	return output
}

// SectionHeading marks off the blocks a tab is made of, the tab being
// several things stacked rather than one table. The label sits inside the
// rule the way a fieldset legend sits in its border: a service's replicas
// stack the same labels several times over, and a heading with nothing
// under it doesn't stop the eye.
func SectionHeading(title string, width int) string {
	rule := "── " + title + " "

	if padding := width - utils.DisplayWidth(rule); padding > 0 {
		rule += strings.Repeat("─", padding)
	} else {
		rule += "───"
	}

	return utils.ColoredString(rule, color.FgCyan)
}

// InstanceInfo takes the labels of any identity lines to leave out: the
// services panel stacks this under a service that has already said those.
// The counters underneath are set off by a gap rather than a heading of
// their own: CPU and memory need no label to say what they are, and a
// ruled heading here would carry the same weight as the replica headings
// stacking whole instances, which are a level above it.
func InstanceInfo(instance *commands.Instance, loc Location, publishHost string, omit ...string) string {
	return InstanceIdentity(instance, loc, publishHost, omit...) + "\n\n" + InstanceStats(instance)
}

// InstanceIdentity is what `incus info` prints before the counters, minus
// what's a tab of its own: no config, no profiles list, no snapshot dates.
// Fields an instance may not have - a description, a compose image, a
// health verdict, addresses, proxy devices - are left out rather than
// shown empty.
func InstanceIdentity(instance *commands.Instance, loc Location, publishHost string, omit ...string) string {
	line := func(label, value string) string {
		if value == "" || lo.Contains(omit, label) {
			return ""
		}

		return utils.WithPadding(label+": ", IdentityPadding) + value + "\n"
	}

	output := LocationLines(loc, omit...)
	output += line("Name", instance.Name)
	output += line("Description", strings.Join(strings.Fields(instance.Instance.Description), " "))
	output += line("Status", strings.ToLower(instance.Instance.Status))
	output += line("Type", InstanceType(instance))
	output += line("Image", instance.Image())
	output += line("Health", instance.HealthStatus())
	output += line("Architecture", instance.Instance.Architecture)
	output += line("Created", LocalTime(instance.Instance.CreatedAt))
	output += line("Last used", LocalTime(instance.Instance.LastUsedAt))
	output += line("IPv4", strings.Join(instance.Addresses("inet"), " "))
	output += line("IPv6", strings.Join(instance.Addresses("inet6"), " "))
	output += line("Ports", strings.Join(lo.Map(instance.PublishedPorts(publishHost),
		func(port commands.PublishedPort, _ int) string { return port.Label() }), "  "))

	output += line("Snapshots", strconv.Itoa(len(instance.Instance.Snapshots)))

	return output
}

// LocalTime is blank for a time that was never set: the daemon reports an
// instance that has never run as last used at the Unix epoch.
func LocalTime(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return ""
	}

	return t.Local().Format(DateTimeFormat)
}

func InstanceStats(instance *commands.Instance) string {
	state := instance.Instance.State
	if state == nil {
		return ""
	}

	// The nested labels under Disk and Network keep a padding of their own,
	// being a level in.
	padding := 12
	output := ""

	output += utils.WithPadding("CPU: ", IdentityPadding) + FormatCPUUsage(state.CPU) + "\n"
	output += utils.WithPadding("Memory: ", IdentityPadding) + FormatMemoryUsage(state.Memory) + "\n"
	output += utils.WithPadding("Processes: ", IdentityPadding) + fmt.Sprint(state.Processes) + "\n"

	output += "\nDisk:\n"
	if len(state.Disk) == 0 {
		output += "  (no disk usage reported)\n"
	} else {
		for _, name := range sortedKeys(state.Disk) {
			output += "  " + utils.WithPadding(name+": ", padding) + FormatDiskUsage(state.Disk[name]) + "\n"
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

// FormatCPUUsage shows cumulative CPU time, not a percentage: the API
// reports total nanoseconds since start, and `incus info` shows the same.
func FormatCPUUsage(cpu api.InstanceStateCPU) string {
	if cpu.Usage <= 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2fs total", time.Duration(cpu.Usage).Seconds())
}

func FormatMemoryUsage(mem api.InstanceStateMemory) string {
	if mem.Usage <= 0 {
		return "N/A"
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

func FormatDiskUsage(disk api.InstanceStateDisk) string {
	if disk.Usage <= 0 {
		return "N/A"
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

func InstanceConfig(instance *commands.Instance) string {
	data, err := utils.MarshalIntoYaml(instance.Instance)
	if err != nil {
		return fmt.Sprintf("Error marshalling instance details: %v", err)
	}

	return utils.ColoredYamlString(string(data))
}

// InstanceEnv reads ExpandedConfig so profile-inherited variables show up too.
func InstanceEnv(instance *commands.Instance) string {
	const prefix = "environment."

	vars := []string{}
	for key, value := range instance.Instance.ExpandedConfig {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		vars = append(vars, strings.TrimPrefix(key, prefix)+"="+value)
	}

	sort.Strings(vars)

	return strings.Join(vars, "\n")
}
