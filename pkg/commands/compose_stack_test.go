package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func TestResolveStackDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"~", "/home/me"},
		{"~/stacks/web/", "/home/me/stacks/web"},
		{"  /srv/web  ", "/srv/web"},
		{"stacks/../web", "/work/web"},
		{".", "/work"},
		// Only a leading `~/` is home; anything else after it is a name.
		{"~other/web", "/work/~other/web"},
	}

	for _, tt := range tests {
		got, err := ResolveStackDir(tt.input, "/work", "/home/me")
		require.NoError(t, err, tt.input)
		assert.Equal(t, tt.want, got, tt.input)
	}

	_, err := ResolveStackDir("  ", "/work", "/home/me")
	assert.Error(t, err)
}

func TestCheckStackDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, CheckStackDir(dir))
	assert.ErrorContains(t, CheckStackDir(file), "not a directory")
	assert.ErrorIs(t, CheckStackDir(filepath.Join(dir, "missing")), os.ErrNotExist)
}

func TestShortenHome(t *testing.T) {
	assert.Equal(t, "~/stacks/web", ShortenHome("/home/me/stacks/web", "/home/me"))
	assert.Equal(t, "~", ShortenHome("/home/me", "/home/me"))
	assert.Equal(t, "/home/meadow/web", ShortenHome("/home/meadow/web", "/home/me"))
	assert.Equal(t, "/srv/web", ShortenHome("/srv/web", ""))
}

func TestStackStatusRollsUpItsInstances(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string][]string
		want     string
	}{
		{"nothing deployed", nil, ServiceNone},
		{"all running", map[string][]string{"web": {"Running", "Running"}, "db": {"Running"}}, "Running"},
		{"all frozen", map[string][]string{"web": {"Frozen"}, "db": {"Frozen"}}, "Frozen"},
		{"one service stopped", map[string][]string{"web": {"Running"}, "db": {"Stopped"}}, ServicePartial},
		{"replicas disagree", map[string][]string{"web": {"Running", "Stopped"}}, ServicePartial},
	}

	for _, tt := range tests {
		stack := &ComposeStack{Services: []ComposeService{{Name: "web"}, {Name: "db"}}, Statuses: tt.statuses}
		assert.Equal(t, tt.want, stack.Status(), tt.name)
	}

	stack := &ComposeStack{
		Services: []ComposeService{{Name: "web"}, {Name: "db"}, {Name: "cache"}},
		Statuses: map[string][]string{"web": {"Running", "Stopped"}, "db": {"Frozen"}},
	}
	assert.Equal(t, []string{ServiceNone, "Frozen", ServicePartial}, stack.ServiceStatuses())
}

func TestGetComposeStatusesGroupsByProjectAndService(t *testing.T) {
	instance := func(project, name, service, status string) api.InstanceFull {
		config := map[string]string{}
		if service != "" {
			config[composeServiceKey] = service
		}

		return api.InstanceFull{Instance: api.Instance{
			Name: name, Project: project, Status: status, ExpandedConfig: config,
		}}
	}

	server := incustest.New(incustest.Server{Instances: []api.InstanceFull{
		instance("web", "web-1", "web", "Running"),
		instance("web", "web-2", "web", "Stopped"),
		instance("web", "db-1", "db", "Running"),
		instance("web", "scratch", "", "Running"),
		instance("default", "app", "", "Running"),
		instance("other", "api-1", "api", "Frozen"),
	}})

	command := NewIncusCommandWithClient(NewDummyLog(), nil, nil, nil, server, "fake")

	statuses, err := command.GetComposeStatuses()
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string][]string{
		"web":   {"web": {"Running", "Stopped"}, "db": {"Running"}},
		"other": {"api": {"Frozen"}},
	}, statuses)
}
