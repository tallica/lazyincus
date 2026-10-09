package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

// F asks for the path, starting it at the root.
func TestEditFileAsksForThePath(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.press(t, 'F')
	s.settle(t, "Edit a file in a-name-long-enough-to-be-cut-off")
	s.settle(t, "│/")
}

func TestEditFileRefusedOnAReadOnlyRemote(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	})
	s.ready(t)

	s.press(t, 'F')
	s.settle(t, "fake is read-only")
}

// The file is named in the instance's project and on its remote, a path
// typed without its leading slash taken as having one.
func TestEditFileCommand(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	instance := &commands.Instance{Name: "web", Project: "shop", Remote: "pve01"}

	for _, path := range []string{"/etc/hosts", "etc/hosts"} {
		cmd := s.gui.instanceFileEditCmd(instance, path)

		assert.Equal(t, []string{"incus", "--project", "shop", "file", "edit", "web/etc/hosts"}, cmd.Args, path)
		assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01", path)
	}
}
