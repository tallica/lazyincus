package presentation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// ServiceInfo is the service's lines, then each instance's Info tab under them.
func ServiceInfo(row *commands.ServiceRow, loc Location, verdict string, project *commands.ComposeProject,
	instanceInfo func(instance *commands.Instance, omit ...string) string,
	width int, tr *i18n.TranslationSet,
) string {
	service := row.Service
	// The same column the instance blocks below use: the tab is one run of
	// labels, the service's and then its instances'.
	padding := IdentityPadding

	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := LocationLines(loc)
	output += line("Service", service.Name)
	output += line("Image", service.ResolvedImage())
	output += line("Registry", verdict)
	output += line("Replicas", ServiceReplicas(service))
	output += line("Health", service.Health())

	if project != nil {
		output += line("Healthcheck", ComposeHealthcheck(project, tr))
	}

	output += line("Command", service.Command)
	output += line("Restart", service.Restart)
	output += line("Ports", strings.Join(service.Ports, ", "))
	output += line("Volumes", strings.Join(service.Volumes, ", "))
	output += line("Devices", strings.Join(service.Devices, ", "))
	output += line("Depends on", strings.Join(service.DependsOn, ", "))

	if len(service.Instances) == 0 {
		return output + "\n" + tr.ServiceNotRunning
	}

	// Each instance's own Info tab under that - the selected replica's
	// alone, or every one of them from the service's own row - each under a
	// heading of its own, and minus the lines the service or that heading
	// has just said: where it lives and its image are the same for every
	// replica, the name and which replica it is are the heading's, and a
	// lone instance's health is what the service's rolls up to.
	omit := []string{"Remote", "Project", "Stack", "Service", "Image", "Name"}
	if len(service.Instances) == 1 {
		omit = append(omit, "Health")
	}

	for _, instance := range row.Instances() {
		output += "\n" + InstanceHeading(service, instance, width, tr) + "\n"
		output += "\n" + instanceInfo(instance.Latest(), omit...)
	}

	return output
}

// InstanceHeading rules off one instance's block and says which of the
// service's instances it is - a name among a dozen identical labels not
// being enough to place you. The number is over the service's instances
// rather than the ones on screen, so a replica's own row still reads "2 of
// 4". A lone instance has no count worth printing, and one carrying the
// service's own name has nothing left to say: the Service line said it.
func InstanceHeading(service *commands.ComposeService, instance *commands.Instance, width int, tr *i18n.TranslationSet) string {
	instances := service.SortedInstances()

	if len(instances) < 2 {
		if instance.Name == service.Name {
			return SectionHeading(tr.InstanceTitle, width)
		}

		return SectionHeading(fmt.Sprintf(tr.ServiceInstanceHeading, instance.Name), width)
	}

	index := slices.IndexFunc(instances, func(other *commands.Instance) bool {
		return other.Name == instance.Name
	})

	return SectionHeading(fmt.Sprintf(
		tr.ServiceReplicaHeading, index+1, len(instances), instance.Name), width)
}

// ServiceNoSingleInstance says which of the two reasons there's no
// instance to show: nothing running, or a row per replica to pick from.
func ServiceNoSingleInstance(service *commands.ComposeService, tr *i18n.TranslationSet) string {
	if len(service.Instances) == 0 {
		return tr.ServiceNotRunning
	}

	return tr.ServiceMultipleInstances
}

// ServiceLogs stacks every instance's log under its heading rather than
// interleaving them - the buffers carry no timestamps to merge on.
func ServiceLogs(service *commands.ComposeService, instances []*commands.Instance, width int,
	logOf func(*commands.Instance) string, tr *i18n.TranslationSet,
) string {
	sections := make([]string, 0, len(instances))

	for _, instance := range instances {
		sections = append(sections, InstanceHeading(service, instance, width, tr)+"\n\n"+strings.TrimRight(logOf(instance), "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// ComposeServiceConfig is the service's slice of `incus-compose config` as YAML.
func ComposeServiceConfig(definition string, found bool, err error, tr *i18n.TranslationSet) string {
	if err != nil {
		return fmt.Sprintf("Error running `incus-compose config`: %v", err)
	}

	if !found {
		return tr.ServiceNotInComposeFile
	}

	return utils.ColoredYamlString(definition)
}

// ServiceConfig is the compose file's half of a service's configuration, then the daemon's.
func ServiceConfig(row *commands.ServiceRow, composeSection string, width int, tr *i18n.TranslationSet) string {
	service := row.Service
	output := SectionHeading(tr.ComposeTitle, width) + "\n\n" + composeSection

	for _, instance := range row.Instances() {
		output += "\n\n" + InstanceHeading(service, instance, width, tr) +
			"\n\n" + InstanceConfig(instance)
	}

	return output
}
