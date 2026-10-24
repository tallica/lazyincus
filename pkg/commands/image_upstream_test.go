package commands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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

	amd64, err := random.Image(64, 2)
	require.NoError(t, err)
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

	return ociFingerprint(layers)
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
	assert.Equal(t, []UpstreamTarget{{Fingerprint: "nginx", Ref: "docker.io/library/nginx:alpine"}}, targets)
}
