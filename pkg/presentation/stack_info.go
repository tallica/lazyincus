package presentation

import (
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
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// StackInstances is one services refresh's view of a stack's project: the
// declared services with their instances, and the orphans no service claims.
type StackInstances struct {
	Remote   string
	Project  string
	Services []*commands.ComposeService
	Orphans  []*commands.Instance
}

// IsOf is whether s is stack's, nil being nobody's.
func (s *StackInstances) IsOf(stack *commands.ComposeStack) bool {
	return s != nil && s.Remote == stack.Remote && s.Project == stack.Name
}

// All is every compose instance in the stack, as the newest listing has it.
func (s *StackInstances) All() []*commands.Instance {
	var instances []*commands.Instance

	for _, service := range s.Services {
		instances = append(instances, service.Instances...)
	}

	instances = append(instances, s.Orphans...)

	return lo.Map(instances, func(instance *commands.Instance, _ int) *commands.Instance {
		return instance.Latest()
	})
}

// StackInfo is laid out the way an instance's Info tab is, with what
// only a stack has - where to reach it, and where the daemon and the
// compose file disagree - under headings of its own.
func StackInfo(stack *commands.ComposeStack, remote, home string, state *StackInstances,
	project *commands.ComposeProject, publishHost string, width int,
	guiConfig *config.GuiConfig, tr *i18n.TranslationSet,
) string {
	output := StackIdentity(stack, remote, home, state, project, guiConfig, tr)
	if stack.Err != nil {
		return output + "\n" + utils.ColoredString(stack.Err.Error(), color.FgRed)
	}

	if !state.IsOf(stack) {
		return output
	}

	section := func(title, body string) string {
		if body == "" {
			return ""
		}

		return "\n" + SectionHeading(title, width) + "\n\n" + body
	}

	instances := state.All()

	output += section(tr.EndpointsTitle, StackEndpoints(state.Services, publishHost))
	output += section(tr.UsageTitle, StackUsage(instances))
	output += section(tr.DriftTitle, StackDrift(state))

	return output
}

func StackIdentity(stack *commands.ComposeStack, remote, home string, state *StackInstances,
	project *commands.ComposeProject, guiConfig *config.GuiConfig, tr *i18n.TranslationSet,
) string {
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", IdentityPadding) + value + "\n"
	}

	listed := []string{}
	if stack.Local {
		listed = append(listed, tr.StackListedLocal)
	}

	if stack.Saved {
		listed = append(listed, tr.StackListedSaved)
	}

	output := line("Remote", remote)
	output += line("Project", stack.Name)
	output += line("Directory", commands.ShortenHome(stack.Dir, home))
	output += line("Listed", strings.Join(listed, ", "))

	if stack.Err != nil {
		return output
	}

	if stack.StatusErr != nil {
		return output + line("Status", utils.ColoredString(stack.StatusErr.Error(), color.FgRed))
	}

	if stack.StatusPending {
		return output + line("Status", utils.ColoredString(fmt.Sprintf(tr.ConnectingTo, stack.Remote), color.FgYellow))
	}

	output += line("Status", DisplayRolledUpStatus(guiConfig, stack.Status()))
	output += line("Services", StackServices(stack))

	if count := lo.Sum(lo.Map(lo.Values(stack.Statuses), func(statuses []string, _ int) int {
		return len(statuses)
	})); count > 0 {
		output += line("Instances", strconv.Itoa(count))
	}

	if !state.IsOf(stack) {
		return output
	}

	instances := state.All()

	output += line("Health", commands.RollUpHealth(instances))

	if project != nil && project.Name == stack.Name {
		output += line("Healthcheck", ComposeHealthcheck(project, tr))
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

	output += line("Created", LocalTime(created))
	output += line("Last used", LocalTime(lastUsed))

	if len(instances) > 0 {
		output += line("Snapshots", strconv.Itoa(snapshots))
	}

	return output
}

func ComposeHealthcheck(project *commands.ComposeProject, tr *i18n.TranslationSet) string {
	if !project.HealthcheckEnabled() {
		return tr.No
	}

	if scope := project.HealthcheckScope(); scope != "" {
		return fmt.Sprintf("%s (scope: %s)", tr.Yes, scope)
	}

	return tr.Yes
}

// StackLogs stacks every service's logs, each instance under a heading of
// its own - a replica's name, or a lone instance's service's, as Endpoints
// labels them.
func StackLogs(stack *commands.ComposeStack, state *StackInstances, width int,
	logOf func(*commands.Instance) string, tr *i18n.TranslationSet,
) string {
	if stack.Err != nil {
		return utils.ColoredString(stack.Err.Error(), color.FgRed)
	}

	if !state.IsOf(stack) {
		return ""
	}

	var sections []string

	for _, service := range SortedServices(state.Services) {
		instances := service.SortedInstances()

		for _, instance := range instances {
			heading := service.Name
			if len(instances) > 1 {
				heading = instance.Name
			}

			sections = append(sections, SectionHeading(heading, width)+"\n\n"+strings.TrimRight(logOf(instance), "\n"))
		}
	}

	if len(sections) == 0 {
		return tr.StackNotRunning
	}

	return strings.Join(sections, "\n\n")
}

// StackServices counts the declared services by their rolled-up status,
// running first: "3 running, 1 stopped".
func StackServices(stack *commands.ComposeStack) string {
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

// StackEndpoints is one line per instance with an address or a published
// port: its address, then the ports. A replica goes by its own name, a
// lone instance by its service's.
func StackEndpoints(services []*commands.ComposeService, host string) string {
	type endpoint struct{ label, address, ports string }

	var endpoints []endpoint

	for _, service := range SortedServices(services) {
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

			endpoints = append(endpoints, endpoint{EndpointName(service, instance), address, ports})
		}
	}

	labelWidth := IdentityPadding
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

// EndpointName is what an instance goes by where a stack lists it: a
// replica by its own name, a lone instance by its service's.
func EndpointName(service *commands.ComposeService, instance *commands.Instance) string {
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

func SortedServices(services []*commands.ComposeService) []*commands.ComposeService {
	return slices.SortedFunc(slices.Values(services), func(a, b *commands.ComposeService) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// StackUsage is an instance's counters summed over the stack, then its
// disks and networks. There's no memory total: each instance reports its own
// limit, and their sum is a figure nobody set.
func StackUsage(instances []*commands.Instance) string {
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

	rootUsage := FormatDiskUsage(root)
	if root.Usage > 0 && len(instances) > 1 {
		rootUsage += fmt.Sprintf(" over %d instances", len(instances))
	}

	disks := map[string]string{"root": rootUsage}
	for volume, usage := range volumes {
		disks[volume] = FormatDiskUsage(usage)
	}

	counts := map[string]string{}
	for network, count := range networks {
		counts[network] = pluralInstances(count)
	}

	// One value column for the section: a volume name too long for the
	// usual one moves the counters' values over with it.
	column := IdentityPadding
	for _, label := range append(slices.Collect(maps.Keys(disks)), slices.Collect(maps.Keys(counts))...) {
		column = max(column, len("  ")+utils.DisplayWidth(label+": "))
	}

	output := utils.WithPadding("CPU: ", column) + FormatCPUUsage(cpu) + "\n"
	output += utils.WithPadding("Memory: ", column) + FormatMemoryUsage(memory) + "\n"
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

// StackDrift is where the daemon and the compose file disagree: compose
// instances of a service the file doesn't declare, and a declared service
// with no instances.
func StackDrift(state *StackInstances) string {
	drift := map[string]string{}

	for service, instances := range lo.GroupBy(state.Orphans, func(instance *commands.Instance) string {
		return instance.ComposeService()
	}) {
		status := commands.RollUpStatus(lo.Map(instances, func(instance *commands.Instance, _ int) string {
			return strings.ToLower(instance.Latest().Status())
		}))

		drift[service] = pluralInstances(len(instances)) + ", " + status + ", not in the compose file"
	}

	for _, service := range state.Services {
		if len(service.Instances) == 0 {
			drift[service.Name] = "declared, not created"
		}
	}

	if len(drift) == 0 {
		return ""
	}

	width := IdentityPadding
	for service := range drift {
		width = max(width, utils.DisplayWidth(service)+2)
	}

	output := ""
	for _, service := range slices.Sorted(maps.Keys(drift)) {
		output += utils.WithPadding(service+": ", width) + drift[service] + "\n"
	}

	return output
}
