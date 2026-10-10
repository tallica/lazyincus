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

// ; on a service offers its name after the cursor, for the verb before it.
func TestTypedComposeCommandOffersTheService(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = "fake"

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, nil, stack)(s)
		labelled(s, map[string]string{"web": "web"})
	})
	s.settle(t, "Services (default)")

	s.press(t, '2')
	s.press(t, ';')
	s.settle(t, "incus-compose: default")
	s.settle(t, "│ web")

	for _, key := range "logs" {
		s.press(t, key)
	}
	s.settle(t, "│logs web")
}

func TestTypedComposeCommandRefusedOnAReadOnlyRemote(t *testing.T) {
	stack := testStack(t, t.TempDir(), "default", "web")
	stack.Remote = "fake"

	s := startScreenWith(t, 140, 40, func(c *config.UserConfig) {
		c.Remotes = map[string]config.RemoteConfig{"fake": {ReadOnly: true}}
	}, withStacks(t, nil, stack))
	s.settle(t, "Services (default)")

	s.press(t, ';')
	s.settle(t, "fake is read-only")
}

// Run in the stack's directory, on its remote.
func TestTypedComposeCmd(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	target := composeTarget{remote: "pve01", dir: "/srv/shop", project: "shop"}

	cmd := s.gui.typedComposeCmd(`incus-compose logs --tail 5 web`, target)
	assert.Equal(t, []string{"incus-compose", "logs", "--tail", "5", "web"}, cmd.Args)
	assert.Equal(t, "/srv/shop", cmd.Dir)
	assert.Contains(t, cmd.Env, "INCUS_REMOTE=pve01")

	assert.Nil(t, s.gui.typedComposeCmd("incus-compose", target))
}
