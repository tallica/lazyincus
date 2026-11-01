package commands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// testRegistry is an in-memory registry counting the manifest GETs it
// serves: Docker Hub's pulls.
type testRegistry struct {
	host  string
	pulls atomic.Int32
}

func newTestRegistry(t *testing.T) *testRegistry {
	t.Helper()

	reg := &testRegistry{}
	handler := registry.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/manifests/") {
			reg.pulls.Add(1)
		}

		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	reg.host = strings.TrimPrefix(server.URL, "http://")

	return reg
}

// push tags a two-platform index and returns the amd64 image's fingerprint
// as Incus would compute it.
func (reg *testRegistry) push(t *testing.T, tag string) string {
	t.Helper()

	fingerprint, _ := reg.pushWith(t, tag, v1.Config{})

	return fingerprint
}

// pushWith is push with the amd64 image's config, returning its manifest's
// digest too: what Incus pins.
func (reg *testRegistry) pushWith(t *testing.T, tag string, config v1.Config) (string, string) {
	t.Helper()

	return reg.pushAnnotated(t, tag, config, nil)
}

// pushAnnotated is pushWith with the amd64 manifest's annotations.
func (reg *testRegistry) pushAnnotated(t *testing.T, tag string, config v1.Config, annotations map[string]string) (string, string) {
	t.Helper()

	amd64, err := random.Image(64, 2)
	require.NoError(t, err)
	amd64, err = mutate.Config(amd64, config)
	require.NoError(t, err)
	if annotations != nil {
		amd64 = mutate.Annotations(amd64, annotations).(v1.Image)
	}
	arm64, err := random.Image(64, 2)
	require.NoError(t, err)

	index := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
		mutate.IndexAddendum{Add: arm64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}},
	)

	ref, err := name.ParseReference(reg.host + "/" + tag)
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(ref, index))

	manifest, err := amd64.Manifest()
	require.NoError(t, err)

	layers := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		layers[i] = layer.Digest.String()
	}

	digest, err := amd64.Digest()
	require.NoError(t, err)

	return ociFingerprint(layers), digest.String()
}

func TestRegistryCheckerFollowsTheTag(t *testing.T) {
	reg := newTestRegistry(t)
	fingerprint := reg.push(t, "library/app:latest")
	checker := NewRegistryChecker()
	target := UpstreamTarget{Fingerprint: fingerprint, Ref: reg.host + "/library/app:latest", Architecture: "x86_64"}

	status := checker.Check(context.Background(), target)
	require.NoError(t, status.Err)
	assert.Equal(t, UpstreamCurrent, status.State)

	// A tag that hasn't moved is a HEAD alone.
	pulls := reg.pulls.Load()
	assert.Equal(t, UpstreamCurrent, checker.Check(context.Background(), target).State)
	assert.Equal(t, pulls, reg.pulls.Load())

	reg.push(t, "library/app:latest")
	assert.Equal(t, UpstreamNewer, checker.Check(context.Background(), target).State)
}

// A newer build says what it is, and what the running one was, from their
// configs.
func TestRegistryCheckerReadsBothBuilds(t *testing.T) {
	reg := newTestRegistry(t)
	fingerprint, pinned := reg.pushWith(t, "library/app:1", v1.Config{
		Labels: map[string]string{labelRevision: "aaa"}, Env: []string{"PATH=/bin", "APP_VERSION=1.0.1"},
	})
	reg.pushWith(t, "library/app:1", v1.Config{
		Labels: map[string]string{labelRevision: "bbb", labelSource: "https://github.com/acme/app"},
		Env:    []string{"PATH=/bin", "APP_VERSION=1.0.2"},
	})

	status := NewRegistryChecker().Check(context.Background(), UpstreamTarget{
		Fingerprint: fingerprint, Ref: reg.host + "/library/app:1", Architecture: "x86_64", Pinned: pinned,
	})
	require.Equal(t, UpstreamNewer, status.State)
	require.NotNil(t, status.From)
	require.NotNil(t, status.To)

	_, from, to := BuildChange(status.Ref, status.From, status.To)
	assert.Equal(t, []string{"1.0.1", "1.0.2"}, []string{from, to})
	assert.Equal(t, "https://github.com/acme/app/compare/aaa...bbb", BuildLink(status.From, status.To))
}

// An official image names its source in the manifest's annotations.
func TestRegistryCheckerReadsAnnotations(t *testing.T) {
	reg := newTestRegistry(t)
	annotated := func(revision string) map[string]string {
		return map[string]string{
			labelRevision: revision,
			labelSource:   "https://github.com/acme/docker-app.git#" + revision + ":alpine",
		}
	}
	fingerprint, pinned := reg.pushAnnotated(t, "library/app:1", v1.Config{}, annotated("aaa"))
	reg.pushAnnotated(t, "library/app:1", v1.Config{}, annotated("bbb"))

	status := NewRegistryChecker().Check(context.Background(), UpstreamTarget{
		Fingerprint: fingerprint, Ref: reg.host + "/library/app:1", Architecture: "x86_64", Pinned: pinned,
	})
	require.Equal(t, UpstreamNewer, status.State)
	assert.Equal(t, "https://github.com/acme/docker-app/compare/aaa...bbb", BuildLink(status.From, status.To))
}

// A running build the registry has since let go is asked for once: Docker
// Hub would count each ask as a pull.
func TestRegistryCheckerAsksForAGoneBuildOnce(t *testing.T) {
	reg := newTestRegistry(t)
	reg.push(t, "library/app:1")

	checker := NewRegistryChecker()
	target := UpstreamTarget{
		Fingerprint: "old", Ref: reg.host + "/library/app:1", Architecture: "x86_64",
		Pinned: "sha256:" + strings.Repeat("0", 64),
	}

	status := checker.Check(context.Background(), target)
	require.Equal(t, UpstreamNewer, status.State)
	assert.Nil(t, status.From)
	assert.NotNil(t, status.To)

	pulls := reg.pulls.Load()
	assert.Nil(t, checker.Check(context.Background(), target).From)
	assert.Equal(t, pulls, reg.pulls.Load())
}

func TestBuildChange(t *testing.T) {
	day := time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local)
	cases := map[string]struct {
		from, to *BuildInfo
		want     []string
	}{
		"the image's own variable first": {
			&BuildInfo{Versions: map[string]string{"GOSU_VERSION": "1.17", "NGINX_VERSION": "1.27.3"}},
			&BuildInfo{Versions: map[string]string{"GOSU_VERSION": "1.18", "NGINX_VERSION": "1.27.4"}},
			[]string{"", "1.27.3", "1.27.4"},
		},
		"another variable, named": {
			&BuildInfo{Versions: map[string]string{"GOSU_VERSION": "1.17", "PG_VERSION": "17.5"}},
			&BuildInfo{Versions: map[string]string{"GOSU_VERSION": "1.17", "PG_VERSION": "17.6"}},
			[]string{"PG_VERSION", "17.5", "17.6"},
		},
		"by label": {
			&BuildInfo{Version: "11.2.0", Versions: map[string]string{"GO_VERSION": "1.27"}},
			&BuildInfo{Version: "11.2.1", Versions: map[string]string{"GO_VERSION": "1.28"}},
			[]string{"", "11.2.0", "11.2.1"},
		},
		"by variable": {
			&BuildInfo{Versions: map[string]string{"NGINX_VERSION": "1.27.3", "NJS_VERSION": "0.8"}},
			&BuildInfo{Versions: map[string]string{"NGINX_VERSION": "1.27.4", "NJS_VERSION": "0.8"}},
			[]string{"", "1.27.3", "1.27.4"},
		},
		"rebuilt the same": {
			&BuildInfo{Version: "7", Created: day},
			&BuildInfo{Version: "7", Created: day.AddDate(0, 0, 9)},
			[]string{"", "2026-10-01", "2026-10-10"},
		},
		"rebuilt the same day": {
			&BuildInfo{Created: day}, &BuildInfo{Created: day.Add(time.Hour)},
			[]string{"", "2026-10-01 09:30", "2026-10-01 10:30"},
		},
		"the old build gone": {nil, &BuildInfo{Version: "2"}, []string{"", "", "2"}},
		"nothing read":       {nil, nil, []string{"", "", ""}},
	}

	for label, c := range cases {
		variable, from, to := BuildChange("docker.io/library/nginx:alpine", c.from, c.to)
		assert.Equal(t, c.want, []string{variable, from, to}, label)
	}
}

func TestOwnVersion(t *testing.T) {
	assert.True(t, ownVersion("docker.io/library/nginx:alpine", "NGINX_VERSION"))
	assert.True(t, ownVersion("docker.io/keinos/sqlite3:latest@sha256:00", "SQLITE_VERSION"))
	assert.True(t, ownVersion("ghcr.io/acme/ic-healthd:1", "IC_HEALTHD_VERSION"))
	assert.False(t, ownVersion("docker.io/gogs/gogs:latest", "GO_VERSION"))
	assert.False(t, ownVersion("docker.io/library/postgres:17", "GOSU_VERSION"))
}

func TestBuildLink(t *testing.T) {
	assert.Equal(t, "https://github.com/acme/app/releases",
		BuildLink(nil, &BuildInfo{Source: "https://github.com/acme/app.git"}))
	assert.Equal(t, "https://gitlab.com/acme/app",
		BuildLink(&BuildInfo{Revision: "a"}, &BuildInfo{Source: "https://gitlab.com/acme/app", Revision: "b"}))
	assert.Equal(t, "https://github.com/acme/app/releases",
		BuildLink(nil, &BuildInfo{Source: "https://github.com/acme/app.git#8fd1238:alpine"}))
	assert.Empty(t, BuildLink(nil, &BuildInfo{Source: "git@github.com:acme/app"}))
	assert.Empty(t, BuildLink(nil, &BuildInfo{}))
}

func TestBuildLinks(t *testing.T) {
	official := &BuildInfo{
		Source:   "https://github.com/redis/docker-library-redis.git#8fd1238:alpine",
		URL:      "https://hub.docker.com/_/redis",
		Versions: map[string]string{"REDIS_VERSION": "8.10.3"},
	}
	assert.Equal(t, []string{
		"https://github.com/redis/redis/releases/tag/8.10.3",
		"https://github.com/redis/redis/releases/tag/v8.10.3",
		"https://hub.docker.com/_/redis",
	}, BuildLinks("docker.io/library/redis:alpine", &BuildInfo{Revision: "a"}, official))

	assert.Equal(t, []string{
		"https://github.com/postgres/postgres/releases/tag/17.2",
		"https://github.com/postgres/postgres/releases/tag/v17.2",
		"https://github.com/docker-library/postgres/releases",
	}, BuildLinks("docker.io/library/postgres:17", nil, &BuildInfo{
		Source: "https://github.com/docker-library/postgres.git#abc:17/bookworm", Version: "17.2",
	}))

	// Packaged off GitHub, there's no project to guess.
	assert.Equal(t, []string{"https://hub.docker.com/_/ubuntu"}, BuildLinks("docker.io/library/ubuntu:24.04", nil, &BuildInfo{
		Source: "https://git.launchpad.net/cloud-images/+oci/ubuntu-base#abc:oci", URL: "https://hub.docker.com/_/ubuntu",
		Versions: map[string]string{"UBUNTU_VERSION": "24.04"},
	}))
	assert.Equal(t, []string{"https://git.launchpad.net/cloud-images/+oci/ubuntu-base"}, BuildLinks("docker.io/library/ubuntu:24.04", nil, &BuildInfo{
		Source: "https://git.launchpad.net/cloud-images/+oci/ubuntu-base#abc:oci",
	}))

	// An image labelled with its own repository keeps the comparison.
	assert.Equal(t, []string{"https://github.com/acme/app/compare/a...b"},
		BuildLinks("ghcr.io/acme/app:1", &BuildInfo{Revision: "a"}, &BuildInfo{Source: "https://github.com/acme/app", Revision: "b"}))
	assert.Empty(t, BuildLinks("ghcr.io/acme/app:1", nil, &BuildInfo{}))
}

func TestFirstLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/there" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	assert.Equal(t, server.URL+"/there", FirstLink(ctx, []string{server.URL + "/gone", server.URL + "/there", "fallback"}))
	assert.Equal(t, "fallback", FirstLink(ctx, []string{server.URL + "/gone", "fallback"}))
}

func TestRegistryCheckerReportsFailures(t *testing.T) {
	reg := newTestRegistry(t)
	checker := NewRegistryChecker()

	missing := checker.Check(context.Background(), UpstreamTarget{Ref: reg.host + "/library/gone:1", Architecture: "x86_64"})
	assert.Equal(t, UpstreamFailed, missing.State)
	assert.Error(t, missing.Err)

	unknownArch := checker.Check(context.Background(), UpstreamTarget{Ref: reg.host + "/library/gone:1", Architecture: "mips"})
	assert.Equal(t, UpstreamFailed, unknownArch.State)
}

func TestOCIRef(t *testing.T) {
	cases := map[string]struct {
		source *api.ImageSource
		want   string
	}{
		"pulled": {
			&api.ImageSource{Protocol: "oci", Server: "https://docker.io", Alias: "library/nginx:alpine@sha256:138a"},
			"docker.io/library/nginx:alpine",
		},
		"unpinned": {
			&api.ImageSource{Protocol: "oci", Server: "https://ghcr.io", Alias: "lxc/incus-compose/ic-healthd:1.3.5"},
			"ghcr.io/lxc/incus-compose/ic-healthd:1.3.5",
		},
		"copied from the server": {
			&api.ImageSource{Protocol: "incus", Server: "https://192.0.2.1:8443", Alias: "e294e334328f"},
			"",
		},
		"none": {nil, ""},
	}

	for label, c := range cases {
		assert.Equal(t, c.want, ociRef(c.source), label)
	}
}

// A project's copy of an image learns its reference from the copy Incus
// pulled, and an image nothing was created from isn't checked.
func TestUpstreamTargets(t *testing.T) {
	pulled := &api.ImageSource{Protocol: "oci", Server: "https://docker.io", Alias: "library/nginx:alpine@sha256:138a"}
	copied := &api.ImageSource{Protocol: "incus", Server: "https://192.0.2.1:8443", Alias: "nginx"}

	server := incustest.New(incustest.Server{
		Instances: []api.InstanceFull{{Instance: api.Instance{
			Name: "web", Project: "stack",
			InstancePut: api.InstancePut{Config: map[string]string{"volatile.base_image": "nginx"}},
		}}},
		Images: []api.Image{
			{Fingerprint: "nginx", Project: "incus-compose-cache", UpdateSource: pulled},
			{Fingerprint: "nginx", Project: "stack", UpdateSource: copied},
			{Fingerprint: "spare", Project: "incus-compose-cache", UpdateSource: pulled},
			{Fingerprint: "built", Project: "stack"},
		},
	})

	log := NewDummyLog()
	command := NewIncusCommandWithClient(log, NewDummyOSCommand(), i18n.NewTranslationSet(log, "en"),
		NewDummyAppConfig(), server, "fake")
	command.UseProject("stack")

	targets, err := command.UpstreamTargets()
	require.NoError(t, err)
	assert.Equal(t, []UpstreamTarget{{Fingerprint: "nginx", Ref: "docker.io/library/nginx:alpine", Pinned: "sha256:138a"}}, targets)
}
