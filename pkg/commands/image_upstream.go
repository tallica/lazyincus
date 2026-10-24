package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/lxc/incus/v7/shared/api"
)

// UpstreamTarget is an OCI image to check against the registry it was
// pulled from.
type UpstreamTarget struct {
	Fingerprint  string
	Ref          string
	Architecture string
	// Pinned is the platform manifest's digest the tag resolved to when
	// pulled, empty when the pull didn't record one.
	Pinned string
}

type UpstreamState int

const (
	UpstreamCurrent UpstreamState = iota + 1
	UpstreamNewer
	UpstreamFailed
)

// UpstreamStatus is what the registry said of one image's tag.
type UpstreamStatus struct {
	State UpstreamState
	Ref   string
	// Digest is the tag's, as the registry had it.
	Digest    string
	Err       error
	CheckedAt time.Time
	// From and To are the build running and the tag's, read for a newer
	// build only; nil when the registry wouldn't say.
	From, To *BuildInfo
}

// MostTelling is the check that says most about several images taken
// together: a newer build if any has one, else a failure, else current.
func MostTelling(updates map[string]UpstreamStatus, fingerprints ...string) UpstreamStatus {
	var best UpstreamStatus

	rank := map[UpstreamState]int{UpstreamCurrent: 1, UpstreamFailed: 2, UpstreamNewer: 3}

	for _, fingerprint := range fingerprints {
		status, ok := updates[fingerprint]
		if ok && rank[status.State] > rank[best.State] {
			best = status
		}
	}

	return best
}

// UpstreamTargets are the OCI images instances were created from, each with
// the reference Incus pulled it by. That reference is on the image's
// update_source, but only on the copy Incus pulled (incus-compose's lives in
// incus-compose-cache); a project's copy points at the Incus server instead.
// So every project's images are read for it, falling back to the listed
// ones for a client limited to some projects.
func (c *IncusCommand) UpstreamTargets() ([]UpstreamTarget, error) {
	images, err := c.GetImages()
	if err != nil {
		return nil, err
	}

	client := c.Client()

	sources, err := client.GetImagesAllProjects()
	if err != nil {
		sources = make([]api.Image, len(images))
		for i, image := range images {
			sources[i] = image.Image
		}
	}

	refs := map[string]*api.ImageSource{}

	for _, source := range sources {
		if ociRef(source.UpdateSource) != "" {
			refs[source.Fingerprint] = source.UpdateSource
		}
	}

	seen := map[string]bool{}
	targets := []UpstreamTarget{}

	for _, image := range images {
		source := refs[image.Fingerprint]
		if source == nil || image.IsUnused() || seen[image.Fingerprint] {
			continue
		}

		_, pinned, _ := strings.Cut(source.Alias, "@")

		seen[image.Fingerprint] = true
		targets = append(targets, UpstreamTarget{
			Fingerprint:  image.Fingerprint,
			Ref:          ociRef(source),
			Architecture: image.Image.Architecture,
			Pinned:       pinned,
		})
	}

	return targets, nil
}

// ociRef is the reference an OCI pull was made by: the registry from the
// server URL, the repository and tag from the alias, less the digest the
// tag resolved to at the time.
func ociRef(source *api.ImageSource) string {
	if source == nil || source.Protocol != "oci" {
		return ""
	}

	server, err := url.Parse(source.Server)
	if err != nil || server.Host == "" {
		return ""
	}

	repository, _, _ := strings.Cut(source.Alias, "@")

	return server.Host + strings.TrimSuffix(server.Path, "/") + "/" + repository
}

// ociPlatforms maps Incus's architecture names onto OCI's.
var ociPlatforms = map[string]v1.Platform{
	"x86_64":  {OS: "linux", Architecture: "amd64"},
	"i686":    {OS: "linux", Architecture: "386"},
	"aarch64": {OS: "linux", Architecture: "arm64"},
	"armv7l":  {OS: "linux", Architecture: "arm", Variant: "v7"},
	"armv6l":  {OS: "linux", Architecture: "arm", Variant: "v6"},
	"ppc64le": {OS: "linux", Architecture: "ppc64le"},
	"s390x":   {OS: "linux", Architecture: "s390x"},
	"riscv64": {OS: "linux", Architecture: "riscv64"},
}

// RegistryChecker asks registries whether a tag still points at the build an
// image was made from. It remembers what each tag's digest was found to
// hold, so a tag that hasn't moved costs a HEAD, which Docker Hub doesn't
// count against its pull limit.
type RegistryChecker struct {
	options []remote.Option

	mu sync.Mutex
	// seen is the fingerprint each tag's digest was found to hold, by ref
	// and architecture.
	seen map[string]seenTag
	// builds are the configs read, by repository and digest: neither moves.
	builds map[string]*BuildInfo
}

type seenTag struct {
	digest      string
	fingerprint string
}

// upstreamParallel caps the registry requests in flight at once.
const upstreamParallel = 4

func NewRegistryChecker(options ...remote.Option) *RegistryChecker {
	return &RegistryChecker{
		options: append([]remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}, options...),
		seen:    map[string]seenTag{},
		builds:  map[string]*BuildInfo{},
	}
}

// CheckAll checks every target, a few at a time, by fingerprint.
func (c *RegistryChecker) CheckAll(ctx context.Context, targets []UpstreamTarget) map[string]UpstreamStatus {
	results := make([]UpstreamStatus, len(targets))
	slots := make(chan struct{}, upstreamParallel)

	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			results[i] = c.Check(ctx, target)
		})
	}

	wg.Wait()

	byFingerprint := make(map[string]UpstreamStatus, len(targets))
	for i, target := range targets {
		byFingerprint[target.Fingerprint] = results[i]
	}

	return byFingerprint
}

func (c *RegistryChecker) Check(ctx context.Context, target UpstreamTarget) UpstreamStatus {
	status := UpstreamStatus{Ref: target.Ref, CheckedAt: time.Now()}

	fingerprint, digest, err := c.tagFingerprint(ctx, target)
	if err != nil {
		status.State, status.Err = UpstreamFailed, err
		return status
	}

	status.Digest = digest
	if fingerprint == target.Fingerprint {
		status.State = UpstreamCurrent
		return status
	}

	status.State = UpstreamNewer
	status.To = c.build(ctx, target, digest)

	if target.Pinned != "" {
		status.From = c.build(ctx, target, target.Pinned)
	}

	return status
}

// build is the config of the target's repository's build at digest, nil
// when it can't be read - a registry may have let an old build go.
func (c *RegistryChecker) build(ctx context.Context, target UpstreamTarget, digest string) *BuildInfo {
	ref, err := name.ParseReference(target.Ref)
	if err != nil {
		return nil
	}

	at := ref.Context().Digest(digest)
	key := buildKey(at, target.Architecture)

	c.mu.Lock()
	info, ok := c.builds[key]
	c.mu.Unlock()

	if ok {
		return info
	}

	options := append([]remote.Option{
		remote.WithContext(ctx), remote.WithPlatform(ociPlatforms[target.Architecture]),
	}, c.options...)

	image, err := remote.Image(at, options...)
	if err == nil {
		info, err = buildInfo(image)
	}

	if err != nil {
		return nil
	}

	c.mu.Lock()
	c.builds[key] = info
	c.mu.Unlock()

	return info
}

// tagFingerprint is the fingerprint Incus would give what the tag points at
// now, for the target's architecture, and the tag's digest.
func (c *RegistryChecker) tagFingerprint(ctx context.Context, target UpstreamTarget) (string, string, error) {
	platform, ok := ociPlatforms[target.Architecture]
	if !ok {
		return "", "", fmt.Errorf("no OCI platform for architecture %q", target.Architecture)
	}

	ref, err := name.ParseReference(target.Ref)
	if err != nil {
		return "", "", err
	}

	options := append([]remote.Option{remote.WithContext(ctx), remote.WithPlatform(platform)}, c.options...)

	head, err := remote.Head(ref, options...)
	if err != nil {
		return "", "", err
	}

	key := target.Ref + " " + target.Architecture
	digest := head.Digest.String()

	c.mu.Lock()
	seen, ok := c.seen[key]
	c.mu.Unlock()

	if ok && seen.digest == digest {
		return seen.fingerprint, digest, nil
	}

	image, err := remote.Image(ref, options...)
	if err != nil {
		return "", "", err
	}

	manifest, err := image.Manifest()
	if err != nil {
		return "", "", err
	}

	layers := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		layers[i] = layer.Digest.String()
	}

	fingerprint := ociFingerprint(layers)
	// A blob, which unlike a manifest Docker Hub doesn't count as a pull.
	info, infoErr := buildInfo(image)

	c.mu.Lock()
	c.seen[key] = seenTag{digest: digest, fingerprint: fingerprint}
	if infoErr == nil {
		c.builds[buildKey(ref.Context().Digest(digest), target.Architecture)] = info
	}
	c.mu.Unlock()

	return fingerprint, digest, nil
}

func buildKey(at name.Digest, architecture string) string {
	return at.String() + " " + architecture
}

// ociFingerprint is how Incus fingerprints an OCI image: the SHA-256 of its
// layer digests strung together, not any digest the registry reports
// (computeFingerprint, client/oci_images.go).
func ociFingerprint(layers []string) string {
	hash := sha256.New()
	for _, layer := range layers {
		hash.Write([]byte(layer))
	}

	return fmt.Sprintf("%x", hash.Sum(nil))
}
