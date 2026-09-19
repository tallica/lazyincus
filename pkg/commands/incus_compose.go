package commands

// ComposeProject is an Incus project that incus-compose created and
// manages. Every such project carries composeManagedKey in its config,
// which is what distinguishes it from a plain Incus project and from
// incus-compose-cache - a project incus-compose also creates, to hold
// pulled images, but never marks.
type ComposeProject struct {
	Name string

	// Local is true when this project matches the compose file in the
	// directory lazyincus was started from - see (*Gui).localComposeProjectName
	// and CLAUDE.md's "Compose" section for what that gates.
	Local bool
}

const composeManagedKey = "user.incus-compose.managed"

// isComposeManagedProject reports whether an Incus project's config marks it
// as one incus-compose created.
func isComposeManagedProject(config map[string]string) bool {
	return config[composeManagedKey] == "true"
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

		result = append(result, &ComposeProject{Name: project.Name})
	}

	return result, nil
}
