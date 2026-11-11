package commands

import (
	"errors"
	"strings"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

// OCIRebuild is the registry and reference an OCI image was pulled by.
type OCIRebuild struct {
	Server string
	// Ref is the repository and tag, no registry: library/nginx:1.27.
	Ref string
}

// Image is the reference written out in full, docker.io/library/nginx:1.27.
func (r OCIRebuild) Image() string {
	return strings.TrimPrefix(strings.TrimPrefix(r.Server, "https://"), "http://") + "/" + r.Ref
}

func (r OCIRebuild) WithTag(tag string) OCIRebuild {
	r.Ref = withoutTag(r.Ref) + ":" + tag

	return r
}

// ErrNoOCISource is an instance whose image no longer says where it came from.
var ErrNoOCISource = errors.New("its image isn't on the server any more to say which registry it came from")

// OCIRebuildSource is read off the image: the instance keeps only its fingerprint.
func (i *Instance) OCIRebuildSource() (OCIRebuild, error) {
	image, _, err := i.Client.GetImage(i.BaseImage())
	if err != nil || ociRef(image.UpdateSource) == "" {
		return OCIRebuild{}, ErrNoOCISource
	}

	ref, _, _ := strings.Cut(image.UpdateSource.Alias, "@")

	return OCIRebuild{Server: image.UpdateSource.Server, Ref: ref}, nil
}

// ErrRebuildSnapshots: docs/Incus.md, "Rebuild".
var ErrRebuildSnapshots = errors.New("incus rebuilds no instance with snapshots")

// Recreate rebuilds from the image it runs, or a fresh pull, stopping a running one for it.
func (i *Instance) Recreate(pull *OCIRebuild) error {
	if len(i.Instance.Snapshots) > 0 {
		return ErrRebuildSnapshots
	}

	running := i.Instance.Status == "Running"

	if running {
		if err := i.updateState("stop", 30, false); err != nil {
			return err
		}
	}

	var err error
	if pull == nil {
		err = i.rebuildFromBaseImage()
	} else {
		err = i.rebuildFromOCI(*pull)
	}

	if running {
		err = errors.Join(err, i.updateState("start", -1, false))
	}

	return err
}

// rebuildFromBaseImage: docs/Incus.md, "Rebuild".
func (i *Instance) rebuildFromBaseImage() error {
	i.Log.Warn().Msgf("rebuilding instance %s from its image %s", i.Name, i.BaseImage())

	op, err := i.Client.RebuildInstanceFromImage(i.Client, api.Image{Fingerprint: i.BaseImage()}, i.Name, api.InstanceRebuildPost{})
	if err != nil {
		return err
	}

	return op.Wait()
}

func (i *Instance) rebuildFromOCI(source OCIRebuild) error {
	server, err := incus.ConnectOCI(source.Server, nil)
	if err != nil {
		return err
	}

	i.Log.Warn().Msgf("rebuilding instance %s from %s", i.Name, source.Image())

	op, err := i.Client.RebuildInstanceFromImage(server, api.Image{Fingerprint: source.Ref, Public: true}, i.Name,
		api.InstanceRebuildPost{Source: api.InstanceSource{Alias: source.Ref}})
	if err != nil {
		return err
	}

	return op.Wait()
}

// BeaconNewerTag is beacon's newer tag for the very image the instance runs.
func (i *Instance) BeaconNewerTag(source OCIRebuild) string {
	verdict := i.Beacon()
	if tag := verdict.NewerTag(); validTag.MatchString(tag) && sameRepository(verdict.Current, source.Image()) {
		return tag
	}

	return ""
}

func sameRepository(a, b string) bool {
	return a != "" && withoutTag(a) == withoutTag(b)
}

func withoutTag(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i]
	}

	return ref
}
