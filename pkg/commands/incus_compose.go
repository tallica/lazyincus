package commands

import "strings"

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
	// directory lazyincus was started from - see (*Gui).localComposeProjectName
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

// GetComposeProjects lists the Incus projects incus-compose manages.
func (c *IncusCommand) GetComposeProjects() ([]*ComposeProject, error) {
	projects, err := c.Client().GetProjects()
	if err != nil {
		return nil, err
	}

	result := make([]*ComposeProject, 0, len(projects))

	for _, project := range projects {
		if !isComposeManagedProject(project.Config) {
			continue
		}

		result = append(result, &ComposeProject{
			Name:        project.Name,
			Description: project.Description,
			Config:      project.Config,
			UsedBy:      project.UsedBy,
		})
	}

	return result, nil
}
