package gui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// `g` asks about the images instances were created from, by the reference
// they were pulled by, and marks the one whose tag has moved on: in its
// row, on its Config tab, and in the image column of what was made from it.
func TestCheckImageUpdates(t *testing.T) {
	var (
		asked []commands.UpstreamTarget
		mutex sync.Mutex
	)

	s := startScreenWith(t, 240, 40, func(userConfig *config.UserConfig) {
		userConfig.Gui.InstanceColumns = []string{"name", "image"}
	}, func(s *screen) {
		images := incustest.Fixture().Images
		images[0].UpdateSource = &api.ImageSource{
			Protocol: "oci", Server: "https://docker.io", Alias: "library/alpine:3.22@sha256:dc92",
		}
		s.server.SetImages(images)

		s.gui.checkUpstreams = func(_ context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
			mutex.Lock()
			asked = append(asked, targets...)
			mutex.Unlock()

			results := map[string]commands.UpstreamStatus{}
			for _, target := range targets {
				results[target.Fingerprint] = commands.UpstreamStatus{State: commands.UpstreamNewer, Ref: target.Ref}
			}

			return results
		}
	})
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Images) })
	s.press(t, 'g')

	screen := s.settle(t, "the tag has a newer build")
	assert.Contains(t, screen, "Upstream:     docker.io/library/alpine:3.22")
	assert.NotEmpty(t, rowWith(screen, "│↑ Alpine 3.22 amd64"))
	assert.NotEmpty(t, rowWith(screen, "│Debian 13 amd64"))
	assert.Contains(t, rowWith(screen, "│web "), "↑")

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, []commands.UpstreamTarget{{
		Fingerprint: "0123456789abcdef0123456789abcdef", Ref: "docker.io/library/alpine:3.22", Pinned: "sha256:dc92",
	}}, asked)
}

// rowWith is the screen's first line containing text.
func rowWith(screen, text string) string {
	for line := range strings.Lines(screen) {
		if strings.Contains(line, text) {
			return line
		}
	}

	return ""
}

// A stack on another remote has its images checked through that remote,
// and the stack and its service say what was found.
func TestCheckImageUpdatesOfAStackElsewhere(t *testing.T) {
	instance := composeFixture("shop", "api-1", "api")
	instance.Config = map[string]string{"volatile.base_image": "aaaa"}
	pve01 := incustest.New(incustest.Server{
		Instances: []api.InstanceFull{instance},
		Images: []api.Image{{Fingerprint: "aaaa", Project: "shop", Architecture: "x86_64", UpdateSource: &api.ImageSource{
			Protocol: "oci", Server: "https://ghcr.io", Alias: "acme/api:1@sha256:00",
		}}},
	})
	pinned := testStack(t, t.TempDir(), "shop", "api")
	pinned.Remote = "pve01"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, pinned)(s)
		withRemotes(map[string]*incustest.Server{"pve01": pve01})(s)
		s.gui.checkUpstreams = func(_ context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
			results := map[string]commands.UpstreamStatus{}
			for _, target := range targets {
				results[target.Fingerprint] = commands.UpstreamStatus{State: commands.UpstreamNewer, Ref: target.Ref}
			}

			return results
		}
	})

	landed(t, s, "stacks")
	require.Eventually(t, func() bool {
		return slices.Equal(serviceNames(t, s), []string{"shop/api"})
	}, 5*time.Second, 20*time.Millisecond)

	s.do(t, func() error { return s.gui.refresh(nil, s.gui.imageUpdatesFetch(false)) })

	s.settle(t, "shop ↑")
	s.settle(t, "api ↑")

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Services) })
	s.settle(t, "Registry:     the tag has a newer build")
}
