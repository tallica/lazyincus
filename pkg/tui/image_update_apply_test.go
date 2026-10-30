package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// updatesModel runs a stack of web, db and cache, each on an image of its
// own, with the registries answering by service; checks counts the asking.
func updatesModel(t *testing.T, states map[string]commands.UpstreamState) (*Model, *[]*exec.Cmd, *int) {
	t.Helper()

	instances := make([]api.InstanceFull, 0, 3)
	for _, service := range []string{"web", "db", "cache"} {
		instance := composeFixture("default", service+"-1", service)
		instance.Config = map[string]string{"volatile.base_image": service + "-image"}
		instances = append(instances, instance)
	}

	server := incustest.New(incustest.Server{Instances: instances})
	m := start(t, options{compose: true, server: server, localStack: testStack(t, t.TempDir(), "default", "web", "db", "cache")}, 140, 40)

	checks := 0
	m.checkUpstreams = func(_ context.Context, _ []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
		checks++
		results := map[string]commands.UpstreamStatus{}
		for service, state := range states {
			results[service+"-image"] = commands.UpstreamStatus{
				State: state, Ref: "docker.io/library/" + service + ":1",
				From: &commands.BuildInfo{Version: "1.0"}, To: &commands.BuildInfo{Version: "1.1"},
			}
		}

		return results
	}

	var ran []*exec.Cmd

	m.runOnTerminal = func(cmd *exec.Cmd, then func() tea.Cmd) tea.Cmd {
		ran = append(ran, cmd)
		return then()
	}

	press(m, "1")
	require.Len(t, m.composeServices(), 3)

	return m, &ran, &checks
}

func menuLabels(m *Model) []string {
	labels := make([]string, 0, len(m.topMenu().items))
	for _, item := range m.topMenu().items {
		labels = append(labels, strings.Join(item.labelColumns, " "))
	}

	return labels
}

// G lists the stack's services with a newer image build, every one ticked,
// and recreates on a pull the ones left ticked - and only those.
func TestUpdateImagesRecreatesTheTicked(t *testing.T) {
	m, ran, checks := updatesModel(t, map[string]commands.UpstreamState{
		"web": commands.UpstreamNewer, "db": commands.UpstreamNewer, "cache": commands.UpstreamCurrent,
	})

	press(m, "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, 1, *checks, "nothing was checked yet")
	assert.Equal(t, []string{"db docker.io/library/db:1 1.0 → 1.1", "web docker.io/library/web:1 1.0 → 1.1"}, menuLabels(m))
	assert.True(t, m.topMenu().items[0].checked && m.topMenu().items[1].checked)

	press(m, "space", "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "pull the newer images of web and recreate")

	press(m, "y")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach", "web"}, (*ran)[0].Args[len((*ran)[0].Args)-7:])
	assert.Equal(t, 2, *checks, "checked again after the update")
}

// A check made already isn't made again; one that said nothing of this
// stack's images is.
func TestUpdateImagesChecksAStackNotYetChecked(t *testing.T) {
	m, _, checks := updatesModel(t, map[string]commands.UpstreamState{"web": commands.UpstreamNewer})
	m.upstreams = map[string]commands.UpstreamStatus{"elsewhere-image": {State: commands.UpstreamCurrent}}

	press(m, "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, 1, *checks)

	press(m, "esc", "G")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, 1, *checks, "checked already")
}

func TestUpdateImagesWithNothingNewer(t *testing.T) {
	m, _, _ := updatesModel(t, map[string]commands.UpstreamState{"web": commands.UpstreamCurrent, "db": commands.UpstreamFailed})

	press(m, "2", "G")
	assert.Nil(t, m.topMenu())
	assert.Contains(t, footer(m), "no service of default has a newer image build")
}

func TestUpdateImagesRefusedReadOnly(t *testing.T) {
	m, ran, _ := updatesModel(t, map[string]commands.UpstreamState{"web": commands.UpstreamNewer})
	m.cfg.ReadOnly = true

	press(m, "G")
	assert.Nil(t, m.topMenu())
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "read-only")
	assert.Empty(t, *ran)
}

func TestBuildChangeStr(t *testing.T) {
	assert.Empty(t, buildChangeStr(commands.UpstreamStatus{}))
	assert.Equal(t, "1.0 → 1.1", buildChangeStr(commands.UpstreamStatus{
		From: &commands.BuildInfo{Version: "1.0"}, To: &commands.BuildInfo{Version: "1.1"},
	}))
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

	m := start(t, options{compose: true, savedStacks: []*commands.ComposeStack{pinned}, remotes: map[string]*incustest.Server{"pve01": pve01}}, 140, 40)

	var asked []commands.UpstreamTarget

	m.checkUpstreams = func(_ context.Context, targets []commands.UpstreamTarget) map[string]commands.UpstreamStatus {
		asked = append(asked, targets...)
		results := map[string]commands.UpstreamStatus{}
		for _, target := range targets {
			results[target.Fingerprint] = commands.UpstreamStatus{State: commands.UpstreamNewer, Ref: target.Ref}
		}

		return results
	}

	press(m, "1")
	require.Equal(t, []string{"pve01"}, m.stackRemotes)
	run(m, refresh(m.imageUpdatesFetch(false)))

	assert.Contains(t, asked, commands.UpstreamTarget{Fingerprint: "aaaa", Ref: "ghcr.io/acme/api:1", Architecture: "x86_64", Pinned: "sha256:00"})
	assert.Equal(t, commands.UpstreamNewer, m.upstreams["aaaa"].State)
}
