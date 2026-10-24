package gui

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
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
		images := fixtureServer().Images
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
	assert.Contains(t, rowWith(screen, "│Alpine 3.22 amd64"), "update")
	assert.NotContains(t, rowWith(screen, "│Debian 13 amd64"), "update")
	assert.Contains(t, rowWith(screen, "│web "), "↑")

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, []commands.UpstreamTarget{{
		Fingerprint: "0123456789abcdef0123456789abcdef", Ref: "docker.io/library/alpine:3.22",
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
