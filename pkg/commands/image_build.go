package commands

import (
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/samber/lo"
)

const (
	labelVersion  = "org.opencontainers.image.version"
	labelSource   = "org.opencontainers.image.source"
	labelRevision = "org.opencontainers.image.revision"
)

// BuildInfo is what an image's config says of its build.
type BuildInfo struct {
	Version  string
	Source   string
	Revision string
	Versions map[string]string
	Created  time.Time
}

func buildInfo(image v1.Image) (*BuildInfo, error) {
	config, err := image.ConfigFile()
	if err != nil {
		return nil, err
	}

	info := &BuildInfo{
		Version:  config.Config.Labels[labelVersion],
		Source:   config.Config.Labels[labelSource],
		Revision: config.Config.Labels[labelRevision],
		Versions: map[string]string{},
		Created:  config.Created.Time,
	}

	for _, variable := range config.Config.Env {
		key, value, _ := strings.Cut(variable, "=")
		if strings.HasSuffix(key, "_VERSION") {
			info.Versions[key] = value
		}
	}

	return info, nil
}

// BuildChange is how two builds of ref's tag differ; variable names the
// `*_VERSION` compared unless it's the image's own. An empty side is unknown.
func BuildChange(ref string, from, to *BuildInfo) (variable, before, after string) {
	if to == nil {
		return "", "", ""
	}

	if from == nil {
		from = &BuildInfo{}
	}

	if to.Version != "" && to.Version != from.Version {
		return "", from.Version, to.Version
	}

	changed := lo.Filter(slices.Sorted(maps.Keys(to.Versions)), func(key string, _ int) bool {
		return to.Versions[key] != from.Versions[key]
	})

	for _, key := range changed {
		if ownVersion(ref, key) {
			return "", from.Versions[key], to.Versions[key]
		}
	}

	if len(changed) > 0 {
		return changed[0], from.Versions[changed[0]], to.Versions[changed[0]]
	}

	return "", buildDate(from.Created, to.Created), buildDate(to.Created, from.Created)
}

// ownVersion is whether key is ref's repository's: NGINX_VERSION for nginx.
func ownVersion(ref, key string) bool {
	repository, _, _ := strings.Cut(ref, "@")
	repository = repository[strings.LastIndex(repository, "/")+1:]
	repository, _, _ = strings.Cut(repository, ":")

	name := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToUpper(repository))

	return key == strings.TrimRight(name, "0123456789")+"_VERSION"
}

// buildDate adds the time when the other build is from the same day.
func buildDate(created, other time.Time) string {
	if created.IsZero() {
		return ""
	}

	created = created.Local()
	if created.Format(time.DateOnly) == other.Local().Format(time.DateOnly) {
		return created.Format("2006-01-02 15:04")
	}

	return created.Format(time.DateOnly)
}

// BuildLink is where to read what changed between two builds, if they say.
func BuildLink(from, to *BuildInfo) string {
	if to == nil || to.Source == "" {
		return ""
	}

	source, err := url.Parse(strings.TrimSuffix(strings.TrimSuffix(to.Source, "/"), ".git"))
	if err != nil || (source.Scheme != "https" && source.Scheme != "http") {
		return ""
	}

	if source.Host != "github.com" {
		return source.String()
	}

	if from != nil && from.Revision != "" && to.Revision != "" && from.Revision != to.Revision {
		return source.String() + "/compare/" + from.Revision + "..." + to.Revision
	}

	return source.String() + "/releases"
}
