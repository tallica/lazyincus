package commands

import (
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// The OCI annotations a build says what it is with, when it says at all.
const (
	labelVersion  = "org.opencontainers.image.version"
	labelSource   = "org.opencontainers.image.source"
	labelRevision = "org.opencontainers.image.revision"
)

// BuildInfo is what an image's config says of the build: what the OCI
// labels name, the `*_VERSION` variables the official images set instead,
// and when it was built.
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

// BuildChange is how two builds of a tag differ, best said: by version
// label, else by a `*_VERSION` variable that changed, else by build date.
// Either side empty is unknown.
func BuildChange(from, to *BuildInfo) (string, string) {
	if to == nil {
		return "", ""
	}

	if from == nil {
		from = &BuildInfo{}
	}

	if to.Version != "" && to.Version != from.Version {
		return from.Version, to.Version
	}

	for _, key := range slices.Sorted(maps.Keys(to.Versions)) {
		if value := to.Versions[key]; value != from.Versions[key] {
			return from.Versions[key], value
		}
	}

	return buildDate(from.Created, to.Created), buildDate(to.Created, from.Created)
}

// buildDate is a build's date, with the time when the other build's is the
// same day.
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

// BuildLink is where to read what changed between two builds: on GitHub, a
// comparison of their revisions, or the releases; elsewhere the source
// itself. Empty when the new build doesn't name its source.
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
