package commands

import (
	"strings"

	"github.com/lxc/incus/v7/shared/api"
)

// ComposeProject is an Incus project that incus-compose created and
// manages. Every such project carries composeManagedKey in its config,
// which is what distinguishes it from a plain Incus project and from
// incus-compose-cache - a project incus-compose also creates, to hold
// pulled images, but never marks.
type ComposeProject struct {
	Name        string
	Description string
	Config      map[string]string

	// UsedBy lists the project's resources as the API returns them
	// (/1.0/instances/web-1?project=..., one entry per instance, image,
	// volume, network and profile) - see ResourceCounts.
	UsedBy []string

	// Local is true when this project matches the compose file in the
	// directory lazyincus was started from - see (*Gui).localComposeProject
	// and CLAUDE.md's "Compose" section for what that gates.
	Local bool
}

const (
	composeManagedKey            = "user.incus-compose.managed"
	projectHealthcheckEnabledKey = "user.healthcheck.enabled"
	projectHealthcheckScopeKey   = "user.healthcheck.scope"
)

// isComposeManagedProject reports whether an Incus project's config marks it
// as one incus-compose created.
func isComposeManagedProject(config map[string]string) bool {
	return config[composeManagedKey] == "true"
}

// HealthcheckEnabled reports whether the project opted every service into
// ic-healthd (the project-wide equivalent of a service's own
// user.healthcheck.enabled).
func (p *ComposeProject) HealthcheckEnabled() bool {
	return p.Config[projectHealthcheckEnabledKey] == "true"
}

// HealthcheckScope is ic-healthd's scope for the project ("global" observed
// in practice), or empty when unset.
func (p *ComposeProject) HealthcheckScope() string {
	return p.Config[projectHealthcheckScopeKey]
}

// ResourceCounts tallies UsedBy by kind - instances, images, volumes,
// networks, profiles - the same categories `incus project show` groups its
// used_by field into.
func (p *ComposeProject) ResourceCounts() map[string]int {
	counts := map[string]int{}

	for _, url := range p.UsedBy {
		if kind := composeResourceKind(url); kind != "" {
			counts[kind]++
		}
	}

	return counts
}

// composeResourceKind extracts the resource kind from one UsedBy URL, e.g.
// "/1.0/instances/web-1?project=x" -> "instances", or
// "/1.0/storage-pools/default/volumes/custom/data?project=x" -> "volumes".
func composeResourceKind(url string) string {
	url = strings.TrimPrefix(url, "/1.0/")
	if idx := strings.Index(url, "?"); idx >= 0 {
		url = url[:idx]
	}

	segments := strings.Split(url, "/")
	if len(segments) == 0 {
		return ""
	}

	switch segments[0] {
	case "instances", "images", "networks", "profiles":
		return segments[0]
	case "storage-pools":
		if len(segments) >= 3 && segments[2] == "volumes" {
			return "volumes"
		}

		return ""
	default:
		return ""
	}
}

// GetProjectInstances lists one project's instances directly, regardless of
// which project the panels are currently scoped to.
func (c *IncusCommand) GetProjectInstances(project string) ([]*Instance, error) {
	client := c.Client().UseProject(project)

	fulls, err := client.GetInstancesFull(api.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	instances := make([]*Instance, len(fulls))

	for i := range fulls {
		full := fulls[i]

		inst := &Instance{
			Name:         full.Name,
			Project:      project,
			Instance:     full.Instance,
			Client:       client,
			OSCommand:    c.OSCommand,
			Log:          c.Log,
			IncusCommand: c,
			Tr:           c.Tr,
		}
		inst.setFull(&full)

		instances[i] = inst
	}

	return instances, nil
}

// ComposeService is one service of the local compose file, together with
// whatever instances the daemon currently holds for it. The service list
// comes from the compose file rather than from the daemon, so a service
// nothing is running still gets an entry - with no instances, and status
// ServiceNone. That's the one thing the panel shows which no instance
// column can.
type ComposeService struct {
	Name string

	// Image and Replicas are what the compose file asked for, not what the
	// daemon ended up with; Instances is the latter.
	Image    string
	Replicas int

	// Project is the Incus project incus-compose created for the stack.
	Project string

	Instances []*Instance
}

// The aggregate states a service's instances roll up to.
const (
	ServiceRunning = "running"
	ServiceStopped = "stopped"
	ServicePartial = "partial"
	ServiceNone    = "none"
)

// Status rolls the service's instances up to one state. Anything that isn't
// running counts as stopped here - a frozen or starting replica alongside a
// running one makes the service partial, which is the distinction that
// matters at this altitude; the per-instance status is a row below.
func (s *ComposeService) Status() string {
	if len(s.Instances) == 0 {
		return ServiceNone
	}

	running := 0

	for _, instance := range s.Instances {
		if instance.IsRunning() {
			running++
		}
	}

	switch running {
	case 0:
		return ServiceStopped
	case len(s.Instances):
		return ServiceRunning
	default:
		return ServicePartial
	}
}

// Health rolls up ic-healthd's per-instance verdict, worst first: one
// unhealthy replica makes the service unhealthy. Empty when no replica has
// been checked, which is also what an instance with no healthcheck reports.
func (s *ComposeService) Health() string {
	worst := ""

	for _, instance := range s.Instances {
		switch instance.HealthStatus() {
		case HealthUnhealthy:
			return HealthUnhealthy
		case HealthStarting:
			worst = HealthStarting
		case HealthHealthy:
			if worst == "" {
				worst = HealthHealthy
			}
		}
	}

	return worst
}

// GetComposeServices pairs the services the compose file declares with the
// project's instances, matching on the label incus-compose stamps on each
// (Instance.ComposeService). Instances whose label names no declared service
// - a one-off from `incus-compose run`, say - belong to no row and are left
// out.
func (c *IncusCommand) GetComposeServices(project string, declared []ComposeService) ([]*ComposeService, error) {
	instances, err := c.GetProjectInstances(project)
	if err != nil {
		return nil, err
	}

	services := make([]*ComposeService, 0, len(declared))
	byName := make(map[string]*ComposeService, len(declared))

	for _, service := range declared {
		service.Project = project
		services = append(services, &service)
		byName[service.Name] = services[len(services)-1]
	}

	for _, instance := range instances {
		if service, ok := byName[instance.ComposeService()]; ok {
			service.Instances = append(service.Instances, instance)
		}
	}

	return services, nil
}

// GetComposeProject fetches the one compose-managed project by name, for the
// header the services panel draws above its rows. A project that exists but
// isn't compose-managed reads as absent: nothing else here would know what
// to do with it.
func (c *IncusCommand) GetComposeProject(name string) (*ComposeProject, error) {
	project, _, err := c.Client().GetProject(name)
	if err != nil {
		return nil, err
	}

	if !isComposeManagedProject(project.Config) {
		return nil, nil
	}

	return &ComposeProject{
		Name:        project.Name,
		Description: project.Description,
		Config:      project.Config,
		UsedBy:      project.UsedBy,
		Local:       true,
	}, nil
}
