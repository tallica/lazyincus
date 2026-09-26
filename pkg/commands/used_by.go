package commands

import (
	"net/url"
	"slices"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
)

// usedByInstanceKeys are the instances a used_by list names outright, as
// Instance.Key has them. An entry without a project is default's: that's
// how the daemon writes those.
func usedByInstanceKeys(usedBy []string) []string {
	keys := []string{}

	for _, entry := range usedBy {
		parsed, err := url.Parse(entry)
		if err != nil {
			continue
		}

		name, ok := strings.CutPrefix(parsed.Path, "/1.0/instances/")
		if !ok || strings.Contains(name, "/") {
			continue
		}

		name, _ = url.PathUnescape(name)

		project := parsed.Query().Get("project")
		if project == "" {
			project = api.ProjectDefaultName
		}

		keys = append(keys, project+"/"+name)
	}

	return keys
}

// IsUsedBy reports whether the instance is on the network: a NIC naming it,
// through a profile or its own devices, or the network's used_by naming the
// instance. used_by alone isn't enough - it names a profile rather than
// every instance with that profile.
func (n *Network) IsUsedBy(instance *Instance) bool {
	if !reachable(n.Network.Project, instance) {
		return false
	}

	for _, device := range instance.Instance.ExpandedDevices {
		if device["type"] == "nic" && (device["network"] == n.Name || device["parent"] == n.Name) {
			return true
		}
	}

	return slices.Contains(usedByInstanceKeys(n.Network.UsedBy), instance.Key())
}

// IsUsedBy reports whether the instance has the volume: a disk device naming
// it, or - for an instance's own volumes - the volume's used_by naming it.
func (v *Volume) IsUsedBy(instance *Instance) bool {
	if !reachable(v.Volume.Project, instance) {
		return false
	}

	if v.IsCustom() {
		for _, device := range instance.Instance.ExpandedDevices {
			if device["type"] == "disk" && device["pool"] == v.Pool && device["source"] == v.Name {
				return true
			}
		}
	}

	return slices.Contains(usedByInstanceKeys(v.Volume.UsedBy), instance.Key())
}

// IsUsedBy reports whether the instance was created from the image.
func (i *Image) IsUsedBy(instance *Instance) bool {
	return slices.Contains(i.UsedBy, instance.Key())
}

// reachable reports whether an instance can use what lives in project: its
// own project's, or default's, which a project without the feature shares.
func reachable(project string, instance *Instance) bool {
	return project == "" || project == api.ProjectDefaultName || project == instance.Project
}
