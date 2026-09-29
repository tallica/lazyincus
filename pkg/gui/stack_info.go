package gui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// stackInstances is one services refresh's view of a stack's project: the
// declared services with their instances, and the orphans no service claims.
type stackInstances struct {
	project  string
	services []*commands.ComposeService
	orphans  []*commands.Instance
}

// all is every compose instance in the stack, as the newest listing has it.
func (s *stackInstances) all() []*commands.Instance {
	var instances []*commands.Instance

	for _, service := range s.services {
		instances = append(instances, service.Instances...)
	}

	instances = append(instances, s.orphans...)

	return lo.Map(instances, func(instance *commands.Instance, _ int) *commands.Instance {
		return instance.Latest()
	})
}

// renderStackInfo ticks for what arrives with the services once the stack
// is selected, and for the counters.
func (gui *Gui) renderStackInfo(stack *commands.ComposeStack) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.stackInfoStr(stack))
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

// stackInfoStr is laid out the way an instance's Info tab is, with what
// only a stack has - where to reach it, and where the daemon and the
// compose file disagree - under headings of its own.
func (gui *Gui) stackInfoStr(stack *commands.ComposeStack) string {
	output := gui.stackIdentityStr(stack)
	if stack.Err != nil {
		return output + "\n" + utils.ColoredString(stack.Err.Error(), color.FgRed)
	}

	state := gui.composeInstances.Load()
	if state == nil || state.project != stack.Name {
		return output
	}

	section := func(title, body string) string {
		if body == "" {
			return ""
		}

		return "\n" + gui.sectionHeading(title) + "\n\n" + body
	}

	instances := state.all()

	output += section(gui.Tr.EndpointsTitle, stackEndpointsStr(state.services, gui.IncusCommand.PublishHost))
	output += section(gui.Tr.UsageTitle, stackUsageStr(instances))
	output += section(gui.Tr.DriftTitle, stackDriftStr(state))

	return output
}

func (gui *Gui) stackIdentityStr(stack *commands.ComposeStack) string {
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", identityPadding) + value + "\n"
	}

	listed := []string{}
	if stack.Local {
		listed = append(listed, gui.Tr.StackListedLocal)
	}

	if stack.Saved {
		listed = append(listed, gui.Tr.StackListedSaved)
	}

	output := line("Project", stack.Name)
	output += line("Directory", commands.ShortenHome(stack.Dir, gui.home))
	output += line("Listed", strings.Join(listed, ", "))

	if stack.Err != nil {
		return output
	}

	output += line("Status", presentation.DisplayRolledUpStatus(&gui.Config.UserConfig.Gui, stack.Status()))
	output += line("Services", stackServicesStr(stack))

	if count := lo.Sum(lo.Map(lo.Values(stack.Statuses), func(statuses []string, _ int) int {
		return len(statuses)
	})); count > 0 {
		output += line("Instances", strconv.Itoa(count))
	}

	state := gui.composeInstances.Load()
	if state == nil || state.project != stack.Name {
		return output
	}

	instances := state.all()

	output += line("Health", commands.RollUpHealth(instances))

	if project := gui.composeProject.Load(); project != nil && project.Name == stack.Name {
		output += line("Healthcheck", gui.composeHealthcheckStr(project))
	}

	var created, lastUsed time.Time

	snapshots := 0

	for _, instance := range instances {
		if at := instance.Instance.CreatedAt; created.IsZero() || at.Before(created) {
			created = at
		}

		if at := instance.Instance.LastUsedAt; at.After(lastUsed) {
			lastUsed = at
		}

		snapshots += len(instance.Instance.Snapshots)
	}

	output += line("Created", localTime(created))
	output += line("Last used", localTime(lastUsed))

	if len(instances) > 0 {
		output += line("Snapshots", strconv.Itoa(snapshots))
	}

	return output
}

// stackServicesStr counts the declared services by their rolled-up status,
// running first: "3 running, 1 stopped".
func stackServicesStr(stack *commands.ComposeStack) string {
	counts := lo.CountValues(lo.Map(stack.ServiceStatuses(), func(status string, _ int) string {
		return strings.ToLower(status)
	}))

	statuses := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		if (a == "running") != (b == "running") {
			if a == "running" {
				return -1
			}

			return 1
		}

		return strings.Compare(a, b)
	})

	return strings.Join(lo.Map(statuses, func(status string, _ int) string {
		return fmt.Sprintf("%d %s", counts[status], status)
	}), ", ")
}

// stackEndpointsStr is one line per instance with an address or a published
// port: its address, then the ports. A replica goes by its own name, a
// lone instance by its service's.
func stackEndpointsStr(services []*commands.ComposeService, host string) string {
	type endpoint struct{ label, address, ports string }

	var endpoints []endpoint

	for _, service := range sortedServices(services) {
		instances := service.SortedInstances()

		for _, instance := range instances {
			instance = instance.Latest()

			address := firstOf(instance.Addresses("inet"), instance.Addresses("inet6"))
			ports := strings.Join(lo.Map(instance.PublishedPorts(host), func(port commands.PublishedPort, _ int) string {
				return port.Label()
			}), "  ")

			if address == "" && ports == "" {
				continue
			}

			endpoints = append(endpoints, endpoint{endpointName(service, instance), address, ports})
		}
	}

	labelWidth := identityPadding
	addressWidth := 0

	for _, endpoint := range endpoints {
		labelWidth = max(labelWidth, utils.DisplayWidth(endpoint.label)+2)
		addressWidth = max(addressWidth, utils.DisplayWidth(endpoint.address))
	}

	output := ""

	for _, endpoint := range endpoints {
		row := utils.WithPadding(endpoint.label+": ", labelWidth) + endpoint.address
		if endpoint.ports != "" && addressWidth > 0 {
			row = utils.WithPadding(row, labelWidth+addressWidth+2)
		}

		row += endpoint.ports

		output += row + "\n"
	}

	return output
}

// endpointName is what an instance goes by where a stack lists it: a
// replica by its own name, a lone instance by its service's.
func endpointName(service *commands.ComposeService, instance *commands.Instance) string {
	if len(service.Instances) > 1 {
		return instance.Name
	}

	return service.Name
}

func firstOf(lists ...[]string) string {
	for _, list := range lists {
		if len(list) > 0 {
			return list[0]
		}
	}

	return ""
}

func sortedServices(services []*commands.ComposeService) []*commands.ComposeService {
	return slices.SortedFunc(slices.Values(services), func(a, b *commands.ComposeService) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// stackUsageStr is an instance's counters summed over the stack, then its
// disks and networks. There's no memory total: each instance reports its own
// limit, and their sum is a figure nobody set.
func stackUsageStr(instances []*commands.Instance) string {
	if len(instances) == 0 {
		return ""
	}

	var cpu api.InstanceStateCPU

	var memory api.InstanceStateMemory

	processes := int64(0)

	// Root disks are summed; a custom volume shared between replicas is
	// counted once, by name.
	var root api.InstanceStateDisk

	volumes := map[string]api.InstanceStateDisk{}
	networks := map[string]int{}

	for _, instance := range instances {
		for _, network := range instance.NetworkNames() {
			networks[network]++
		}

		for device, volume := range instance.CustomVolumes() {
			if _, ok := volumes[volume]; !ok {
				volumes[volume] = api.InstanceStateDisk{}
			}

			if state := instance.Instance.State; state != nil && state.Disk[device].Usage > 0 {
				volumes[volume] = state.Disk[device]
			}
		}

		state := instance.Instance.State
		if state == nil {
			continue
		}

		cpu.Usage += max(state.CPU.Usage, 0)
		memory.Usage += max(state.Memory.Usage, 0)
		memory.UsagePeak += max(state.Memory.UsagePeak, 0)
		processes += max(state.Processes, 0)
		root.Usage += max(state.Disk["root"].Usage, 0)
	}

	rootUsage := formatDiskUsage(root)
	if root.Usage > 0 && len(instances) > 1 {
		rootUsage += fmt.Sprintf(" over %d instances", len(instances))
	}

	disks := map[string]string{"root": rootUsage}
	for volume, usage := range volumes {
		disks[volume] = formatDiskUsage(usage)
	}

	counts := map[string]string{}
	for network, count := range networks {
		counts[network] = pluralInstances(count)
	}

	// One value column for the section: a volume name too long for the
	// usual one moves the counters' values over with it.
	column := identityPadding
	for _, label := range append(slices.Collect(maps.Keys(disks)), slices.Collect(maps.Keys(counts))...) {
		column = max(column, len("  ")+utils.DisplayWidth(label+": "))
	}

	output := utils.WithPadding("CPU: ", column) + formatCPUUsage(cpu) + "\n"
	output += utils.WithPadding("Memory: ", column) + formatMemoryUsage(memory) + "\n"
	output += utils.WithPadding("Processes: ", column) + strconv.FormatInt(processes, 10) + "\n"

	output += "\nDisk:\n" + nestedLines(disks, append([]string{"root"}, slices.Sorted(maps.Keys(volumes))...), column)

	if len(counts) > 0 {
		output += "\nNetwork:\n" + nestedLines(counts, slices.Sorted(maps.Keys(counts)), column)
	}

	return output
}

// nestedLines is a group's entries, indented a level in, the way an
// instance's Disk and Network entries are, their values at column.
func nestedLines(values map[string]string, order []string, column int) string {
	output := ""
	for _, label := range order {
		output += "  " + utils.WithPadding(label+": ", column-len("  ")) + values[label] + "\n"
	}

	return output
}

func pluralInstances(count int) string {
	if count == 1 {
		return "1 instance"
	}

	return fmt.Sprintf("%d instances", count)
}

// stackDriftStr is where the daemon and the compose file disagree: compose
// instances of a service the file doesn't declare, and a declared service
// with no instances.
func stackDriftStr(state *stackInstances) string {
	drift := map[string]string{}

	for service, instances := range lo.GroupBy(state.orphans, func(instance *commands.Instance) string {
		return instance.ComposeService()
	}) {
		status := commands.RollUpStatus(lo.Map(instances, func(instance *commands.Instance, _ int) string {
			return strings.ToLower(instance.Latest().Status())
		}))

		drift[service] = pluralInstances(len(instances)) + ", " + status + ", not in the compose file"
	}

	for _, service := range state.services {
		if len(service.Instances) == 0 {
			drift[service.Name] = "declared, not created"
		}
	}

	if len(drift) == 0 {
		return ""
	}

	width := identityPadding
	for service := range drift {
		width = max(width, utils.DisplayWidth(service)+2)
	}

	output := ""
	for _, service := range slices.Sorted(maps.Keys(drift)) {
		output += utils.WithPadding(service+": ", width) + drift[service] + "\n"
	}

	return output
}
