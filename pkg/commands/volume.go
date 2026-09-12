package commands

import (
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

	Volume    api.StorageVolume
	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

// Key identifies the volume across refreshes and in the panel's context
// cache.
func (v *Volume) Key() string {
	return v.Pool + "/" + v.Volume.Type + "/" + v.Name
}

// IsCustom reports whether this is a volume someone created, rather than one
// Incus manages for an instance, image or snapshot.
func (v *Volume) IsCustom() bool {
	return v.Volume.Type == "custom"
}

func (v *Volume) UsedByCount() int {
	return len(v.Volume.UsedBy)
}

// Delete removes the volume. Incus refuses for volumes backing an instance
// or image, which is what we want: those are deleted with their owner.
func (v *Volume) Delete() error {
	return v.Client.DeleteStoragePoolVolume(v.Pool, v.Volume.Type, v.Name)
}
