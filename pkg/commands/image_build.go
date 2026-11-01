package commands

import (
	"context"
	"maps"
	"net/http"
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
	labelURL      = "org.opencontainers.image.url"
)

// BuildInfo is what an image's config says of its build.
type BuildInfo struct {
	Version  string
	Source   string
	Revision string
	URL      string
	Versions map[string]string
	Created  time.Time
}

func buildInfo(image v1.Image) (*BuildInfo, error) {
	config, err := image.ConfigFile()
	if err != nil {
		return nil, err
	}

	manifest, err := image.Manifest()
	if err != nil {
		return nil, err
	}

	// The official images label their source in the manifest, not the config.
	labelOr := func(key string) string {
		return lo.CoalesceOrEmpty(config.Config.Labels[key], manifest.Annotations[key])
	}

	info := &BuildInfo{
		Version:  config.Config.Labels[labelVersion],
		Source:   labelOr(labelSource),
		Revision: labelOr(labelRevision),
		URL:      labelOr(labelURL),
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

// BuildLinks are where to read what changed, best first; only the last is sure to exist.
func BuildLinks(ref string, from, to *BuildInfo) []string {
	link := BuildLink(from, to)

	source := officialSource(to)
	if source == nil {
		return lo.Compact([]string{link})
	}

	var links []string
	if upstream := packagedUpstream(source); upstream != "" {
		if version := ownBuildVersion(ref, to); version != "" {
			links = append(links, upstream+"/releases/tag/"+version, upstream+"/releases/tag/v"+version)
		}
	}

	return lo.Compact(append(links, lo.CoalesceOrEmpty(to.URL, link)))
}

// officialSource is the source of an official image's build, else nil.
func officialSource(to *BuildInfo) *url.URL {
	if to == nil {
		return nil
	}

	// Only bashbrew, which builds the official images, adds the #commit:dir.
	source, err := url.Parse(to.Source)
	if err != nil || source.Fragment == "" {
		return nil
	}

	return source
}

// packagedUpstream guesses redis/redis from redis/docker-library-redis.
func packagedUpstream(source *url.URL) string {
	if source.Host != "github.com" {
		return ""
	}

	owner, repository, ok := strings.Cut(strings.Trim(strings.TrimSuffix(source.Path, ".git"), "/"), "/")
	if !ok {
		return ""
	}

	project := strings.TrimPrefix(strings.TrimPrefix(repository, "docker-library-"), "docker-")
	if owner == "docker-library" {
		owner = project
	}

	return "https://github.com/" + owner + "/" + project
}

// ownBuildVersion is the build's version of what ref is an image of.
func ownBuildVersion(ref string, info *BuildInfo) string {
	if info.Version != "" {
		return info.Version
	}

	for key, version := range info.Versions {
		if ownVersion(ref, key) {
			return version
		}
	}

	return ""
}

// FirstLink is the first of links that exists, else the last.
func FirstLink(ctx context.Context, links []string) string {
	client := &http.Client{Timeout: 5 * time.Second}

	for _, link := range links[:len(links)-1] {
		request, err := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
		if err != nil {
			continue
		}

		response, err := client.Do(request)
		if err != nil {
			continue
		}
		_ = response.Body.Close()

		if response.StatusCode == http.StatusOK {
			return link
		}
	}

	return links[len(links)-1]
}

// BuildLink is where to read what changed between two builds, if they say.
func BuildLink(from, to *BuildInfo) string {
	if to == nil || to.Source == "" {
		return ""
	}

	source, err := url.Parse(to.Source)
	if err != nil || (source.Scheme != "https" && source.Scheme != "http") {
		return ""
	}
	source.Fragment, source.RawFragment = "", ""
	source.Path = strings.TrimSuffix(strings.TrimSuffix(source.Path, "/"), ".git")

	if source.Host != "github.com" {
		return source.String()
	}

	if from != nil && from.Revision != "" && to.Revision != "" && from.Revision != to.Revision {
		return source.String() + "/compare/" + from.Revision + "..." + to.Revision
	}

	return source.String() + "/releases"
}
