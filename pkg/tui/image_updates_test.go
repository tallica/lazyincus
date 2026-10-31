package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/config"
)

// g asks about the images instances were created from, by the reference
// they were pulled by, and marks the one whose tag has moved on: in its
// row, on its Config tab, and in the image column of what was made from it.
func TestGChecksTheRegistriesAndMarksNewerBuilds(t *testing.T) {
	server := incustest.Fixture()
	images := incustest.Fixture().Images
	images[0].UpdateSource = &api.ImageSource{Protocol: "oci", Server: "https://docker.io", Alias: "library/alpine:3.22@sha256:dc92"}
	server.SetImages(images)

	m := start(t, options{server: server, configure: func(c *config.UserConfig) {
		c.Gui.InstanceColumns = []string{"name", "image"}
	}}, 240, 40)

	var asked []commands.UpstreamTarget
	m.checkUpstreams = func(_ context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
		asked = append(asked, targets...)
		results := map[string]commands.UpstreamStatus{}
		for _, target := range targets {
			results[target.Fingerprint] = commands.UpstreamStatus{State: commands.UpstreamNewer, Ref: target.Ref}
		}

		return results
	}

	press(m, "3", "g")
	assert.Equal(t, []commands.UpstreamTarget{{
		Fingerprint: "0123456789abcdef0123456789abcdef", Ref: "docker.io/library/alpine:3.22", Pinned: "sha256:dc92",
	}}, asked)

	assert.Contains(t, mainText(m), "Upstream:     docker.io/library/alpine:3.22")
	assert.Contains(t, mainText(m), "Registry:     the tag has a newer build")
	rows := rowsOf(m, "images")
	assert.True(t, strings.HasPrefix(rows[0], "↑ Alpine 3.22 amd64"), rows[0])
	assert.True(t, strings.HasPrefix(rows[1], "Debian 13 amd64"), rows[1])
	assert.Contains(t, rowOf(m, "web"), "↑")
	assert.Regexp(t, `^1 with a newer build, 0 current, 0 couldn't be checked`, footer(m))
}
