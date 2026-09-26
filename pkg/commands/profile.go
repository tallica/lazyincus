package commands

import (
	"slices"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Profile is a profile: config and devices instances take on by naming it.
type Profile struct {
	Name string

	Profile   api.Profile
	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

// Key identifies the profile across refreshes: every project has a
// "default".
func (p *Profile) Key() string {
	return p.Profile.Project + "/" + p.Name
}

func (p *Profile) UsedByCount() int {
	return len(p.Profile.UsedBy)
}

// IsUsedBy reports whether the instance has the profile: named in its
// profiles, or in the profile's used_by. A project without
// features.profiles uses default's, so default's profiles reach every such
// project's instances.
func (p *Profile) IsUsedBy(instance *Instance) bool {
	if !reachable(p.Profile.Project, instance) {
		return false
	}

	return slices.Contains(instance.Instance.Profiles, p.Name) ||
		slices.Contains(usedByInstanceKeys(p.Profile.UsedBy), instance.Key())
}

// Delete removes the profile. Incus refuses while anything uses it.
func (p *Profile) Delete() error {
	return p.Client.DeleteProfile(p.Name)
}
