package commands

import (
	"net/url"
	"strings"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Volume is one storage volume within a pool. Volumes are only unique per
// (pool, type, name): an instance and a custom volume can share a name.
type Volume struct {
	Pool string
	Name string

	Volume api.StorageVolume
	// Usage is what the volume takes on disk, nil where the pool's driver
	// can't say - a dir pool without project quotas.
	Usage *api.StorageVolumeStateUsage
	// PoolDriver and PoolSpace describe the pool it's in, PoolSpace nil if
	// the daemon wouldn't say.
	PoolDriver string
	PoolSpace  *api.ResourcesStoragePoolSpace
	Client     incus.InstanceServer
	OSCommand  *OSCommand
	Log        *logrus.Entry
	Tr         *i18n.TranslationSet
}

// Key identifies the volume across refreshes and in the panel's context
// cache. Project included: the all-projects view can hold two volumes with
// the same name in the same pool, one per project.
func (v *Volume) Key() string {
	return v.Volume.Project + "/" + v.Pool + "/" + v.Volume.Type + "/" + v.Name
}

// IsCustom reports whether this is a volume someone created, rather than one
// Incus manages for an instance, image or snapshot.
func (v *Volume) IsCustom() bool {
	return v.Volume.Type == "custom"
}

func (v *Volume) UsedByCount() int {
	return len(v.Volume.UsedBy)
}

// IsOrphaned reports a custom volume nothing has attached, which is either
// kept on purpose or forgotten. The other types always belong to something.
func (v *Volume) IsOrphaned() bool {
	return v.IsCustom() && v.UsedByCount() == 0
}

// Users names what the volume's used_by URLs point at: an instance by its
// name, anything else by its kind and name, and the project when it isn't
// the volume's own.
func (v *Volume) Users() []string {
	users := make([]string, 0, len(v.Volume.UsedBy))

	for _, entry := range v.Volume.UsedBy {
		parsed, err := url.Parse(entry)
		if err != nil {
			users = append(users, entry)
			continue
		}

		kind, name, _ := strings.Cut(strings.TrimPrefix(parsed.Path, "/1.0/"), "/")
		name, _ = url.PathUnescape(name)

		user := name
		if kind != "instances" {
			user = strings.TrimSuffix(kind, "s") + " " + name
		}

		if project := parsed.Query().Get("project"); project != "" && project != v.Volume.Project {
			user += " (" + project + ")"
		}

		users = append(users, user)
	}

	return users
}

// Delete removes the volume. Incus refuses for volumes backing an instance
// or image, which is what we want: those are deleted with their owner.
func (v *Volume) Delete() error {
	return v.Client.DeleteStoragePoolVolume(v.Pool, v.Volume.Type, v.Name)
}
