package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

// : starts from the selected row's project, ready for the verb.
func TestTypedCommandAsksWithTheRowsProject(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	project := onLoop(t, s, func() string {
		instance, _ := s.gui.Panels.Instances.GetSelectedItem()
		return instance.Project
	})

	s.press(t, ':')
	s.settle(t, "enter to run")
	s.settle(t, "│--project "+project+" ")
}

func TestTypedCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	s := startScreen(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	})
	s.ready(t)

	s.press(t, ':')
	s.settle(t, "fake is read-only")
}

// Split on spaces outside quotes, never through a shell, on the remote.
func TestTypedCommandCmd(t *testing.T) {
	s := startScreen(t, 140, 40, nil)

	cmd := s.gui.typedCommandCmd(`incus --project shop exec web -- sh -c "echo hi | wc -c"`, "pve01")
	assert.Equal(t, []string{"incus", "--project", "shop", "exec", "web", "--", "sh", "-c", "echo hi | wc -c"}, cmd.Args)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	assert.Nil(t, s.gui.typedCommandCmd("incus", "pve01"))
	assert.Nil(t, s.gui.typedCommandCmd("   ", "pve01"))
}
