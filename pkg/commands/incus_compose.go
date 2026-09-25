package commands

import (
	"sort"
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

// GetProjectInstances lists one project's instances directly, regardless of
// which project the panels are currently scoped to.
func (c *IncusCommand) GetProjectInstances(project string) ([]*Instance, error) {
	client := c.Client().UseProject(project)
	listing := c.runtimes.beginListing()

	fulls, err := client.GetInstancesFull(api.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	instances := make([]*Instance, len(fulls))

	for i := range fulls {
		instances[i] = c.newInstance(fulls[i], project, client, listing)
	}

	c.runtimes.prune(func(listed string) bool { return listed == project }, instances, listing)

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

	// The rest of what the compose file declares for this service, as the
	// Info tab prints it: already rendered, since only display wants them.
	Command   string
	Restart   string
	Ports     []string
	Volumes   []string
	Devices   []string
	DependsOn []string

	// Project is the Incus project incus-compose created for the stack.
	Project string

	Instances []*Instance
}

// The two states a service can be in that no instance can: its instances
// disagree, or it has none.
const (
	ServicePartial = "partial"
	ServiceNone    = "none"
)

// Status is an instance status: a service is its instances, so a frozen
// one reads frozen here rather than being flattened into stopped. It's the
// daemon's own spelling, the same value the instances panel renders.
// Replicas that disagree make the service ServicePartial, and no instances
// at all ServiceNone.
func (s *ComposeService) Status() string {
	if len(s.Instances) == 0 {
		return ServiceNone
	}

	status := s.Instances[0].Instance.Status

	for _, instance := range s.Instances[1:] {
		if !strings.EqualFold(instance.Instance.Status, status) {
			return ServicePartial
		}
	}

	return status
}

// ResolvedImage is the image reference with its registry host, which the
// compose file's own value may lack (`eclipse-mosquitto:2.1-alpine` in the
// file, `docker.io/library/eclipse-mosquitto:2.1-alpine` once incus-compose
// has pulled it). Every replica came from the same image, so the first one
// answers; until any is running, the file's value is all there is.
func (s *ComposeService) ResolvedImage() string {
	for _, instance := range s.Instances {
		if image := instance.ComposeImage(); image != "" {
			return image
		}
	}

	return s.Image
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

// SortedInstances orders the replicas by name, the order they come back in
// being otherwise whatever the daemon returned.
func (s *ComposeService) SortedInstances() []*Instance {
	instances := append([]*Instance(nil), s.Instances...)
	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })

	return instances
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
		Local:       true,
	}, nil
}

// ServiceRow is one line of the Services panel: a service, or - when it has
// replicas - one of those under it.
type ServiceRow struct {
	Service *ComposeService

	// Instance is the replica the row stands for, and nil on a service's
	// own row.
	Instance *Instance
}

func ServiceRows(services []*ComposeService) []*ServiceRow {
	rows := make([]*ServiceRow, 0, len(services))

	for _, service := range services {
		rows = append(rows, &ServiceRow{Service: service})

		if len(service.Instances) < 2 {
			continue
		}

		for _, instance := range service.SortedInstances() {
			rows = append(rows, &ServiceRow{Service: service, Instance: instance})
		}
	}

	return rows
}

// Key identifies the row across refreshes, which build new ComposeService
// values every time.
func (r *ServiceRow) Key() string {
	if r.Instance == nil {
		return r.Service.Name
	}

	return r.Service.Name + "/" + r.Instance.Name
}

// SelectedInstance is the instance the row means: the replica it stands
// for, or a lone service's only one. A service with replicas means all of
// them, so it answers with none - which is what every caller branches on.
func (r *ServiceRow) SelectedInstance() (*Instance, bool) {
	if r.Instance != nil {
		return r.Instance, true
	}

	if len(r.Service.Instances) == 1 {
		return r.Service.Instances[0], true
	}

	return nil, false
}

// Instances is what the row stands for, for the tabs that show an instance
// each.
func (r *ServiceRow) Instances() []*Instance {
	if r.Instance != nil {
		return []*Instance{r.Instance}
	}

	return r.Service.SortedInstances()
}
