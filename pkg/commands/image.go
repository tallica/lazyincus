package commands

import (
	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Image is a local image on the server, as listed by `incus image list`.
type Image struct {
	// Fingerprint is the image's full SHA-256, and its identity: aliases come
	// and go, and an image without one is addressed by fingerprint alone.
	Fingerprint string

	Image     api.Image
	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

// ShortFingerprint is the 12-character form `incus image list` shows.
func (i *Image) ShortFingerprint() string {
	if len(i.Fingerprint) < 12 {
		return i.Fingerprint
	}

	return i.Fingerprint[:12]
}

// Alias is the first alias, or an empty string for an image that has none -
// which is the common case for images pulled as a dependency of an instance.
func (i *Image) Alias() string {
	if len(i.Image.Aliases) == 0 {
		return ""
	}

	return i.Image.Aliases[0].Name
}

// Description is the image's own description property, which is what
// identifies a cached image in `incus image list` - those arrive without an
// alias.
func (i *Image) Description() string {
	return i.Image.Properties["description"]
}

// Label is the best name available for the image, in the order a reader
// would want it: its alias, the description an unaliased image carries, or
// the fingerprint that always exists.
func (i *Image) Label() string {
	if alias := i.Alias(); alias != "" {
		return alias
	}

	if description := i.Description(); description != "" {
		return description
	}

	return i.ShortFingerprint()
}

func (i *Image) IsVM() bool {
	return i.Image.Type == "virtual-machine"
}

func (i *Image) Delete() error {
	op, err := i.Client.DeleteImage(i.Fingerprint)
	if err != nil {
		return err
	}

	return op.Wait()
}
