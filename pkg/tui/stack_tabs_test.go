package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// stopped is instance, stopped: the fake daemon can't exec into a running one for Top.
func stopped(instance api.InstanceFull) api.InstanceFull {
	instance.Status = "Stopped"
	instance.State = nil

	return instance
}

// replicatedStack is the stack default declaring a service for each distinct label of instances.
func replicatedStack(t *testing.T, instances ...api.InstanceFull) *Model {
	t.Helper()

	var services []string
	for _, instance := range instances {
		if service := instance.ExpandedConfig["user.label.incus-compose.service"]; !slices.Contains(services, service) {
			services = append(services, service)
		}
	}

	server := incustest.New(incustest.Server{Instances: instances})

	return start(t, options{compose: true, server: server, localStack: testStack(t, t.TempDir(), "default", services...)}, 140, 40)
}

func TestTheStackInfoTabNamesItsServicesAndEndpoints(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"), composeFixture("default", "db-1", "db"))

	text := mainText(m)
	assert.Contains(t, text, "Project:")
	assert.Contains(t, text, "Directory:")
	assert.Contains(t, text, "── Endpoints")
	assert.Contains(t, text, "web-1:")
}

// A resize keeps the task, so its next tick has to draw at the new width.
func TestTheStackInfoTabFollowsAResize(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"))
	task := m.stackInfoTask()

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	var content string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task(ctx, &mainWriter{send: func(msg tea.Msg) { content = msg.(mainWriteMsg).content }})

	heading := ""
	for line := range strings.SplitSeq(ansi.Strip(content), "\n") {
		if strings.HasPrefix(line, "── Endpoints") {
			heading = line
		}
	}
	assert.Equal(t, m.main.width, ansi.StringWidth(heading))
}

func TestTheStackInfoTabFollowsAConfigReload(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"))
	require.Contains(t, mainText(m), "Status:       running")

	require.NoError(t, os.WriteFile(m.cfg.ConfigFilename(), []byte("gui:\n  instanceStatusStyle: short\n"), 0o600))
	m.Update(configStatMsg{modTime: time.Unix(1, 0)})
	_, cmd := m.Update(configStatMsg{modTime: time.Unix(2, 0)})
	run(m, cmd)

	assert.Contains(t, mainText(m), "Status:       r\n")
}

// Logs heads each instance with its replica's name, or its service's for a lone one.
func TestStackLogsStackEachInstance(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"), composeFixture("default", "db-1", "db"))

	press(m, "m")
	assert.Equal(t, "main", m.currentView())
	assert.Equal(t, "stacks", m.mainParent)

	text := mainText(m)
	assert.Contains(t, text, "── db ")
	assert.Contains(t, text, "── web-1 ")
	assert.Contains(t, text, "── web-2 ")
	assert.NotContains(t, text, "── web ─")
}

// m names the Logs tab for good: the next tab key moves off it.
func TestTabsSwitchAfterJumpingToLogs(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"))

	press(m, "m", "esc")
	assert.Equal(t, "stacks", m.currentView())
	assert.Equal(t, 1, m.panelNamed("stacks").mainTab)

	press(m, "[")
	assert.Equal(t, 0, m.panelNamed("stacks").mainTab)
	assert.Contains(t, mainText(m), "Directory:")
}

// A replica's row numbers it among its service's replicas.
func TestServiceInfoOnAReplicaNumbersIt(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"))

	press(m, "2", "j", "j")
	require.Equal(t, "web/web-2", m.selectedServiceRow().Key())

	text := mainText(m)
	assert.Contains(t, text, "Service:")
	assert.Contains(t, text, "Replica 2 of 2")
	assert.NotContains(t, text, "Replica 1 of 2")

	press(m, "k", "k")
	text = mainText(m)
	assert.Contains(t, text, "Replica 1 of 2")
	assert.Contains(t, text, "Replica 2 of 2")
}

// A replicated service's Logs tab stacks every replica's, and a replica's is its own.
func TestServiceLogsStackEachReplica(t *testing.T) {
	m := replicatedStack(t, composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"))

	press(m, "2", "m")
	text := mainText(m)
	assert.Contains(t, text, "Replica 1 of 2")
	assert.Contains(t, text, "Replica 2 of 2")

	press(m, "esc", "j")
	assert.NotContains(t, mainText(m), "Replica 2 of 2")
}

// openTab shows p's tab named key, as a key would.
func openTab(m *Model, p *panel, key string) {
	m.setMainTab(p, key)
	_, cmd := m.Update(nil)
	run(m, cmd)
}

// Env and Top are the instance's own, and say why they have none for a service with replicas.
func TestServiceEnvAndTopAreTheInstances(t *testing.T) {
	m := replicatedStack(t, stopped(composeFixture("default", "web-1", "web")), stopped(composeFixture("default", "web-2", "web")))

	press(m, "2")
	require.Equal(t, "web", m.selectedServiceRow().Key())

	p := m.panelNamed("services")
	openTab(m, p, "env")
	assert.Equal(t, m.tr.ServiceMultipleInstances, mainText(m))

	openTab(m, p, "top")
	assert.Equal(t, m.tr.ServiceMultipleInstances, mainText(m))

	press(m, "j")
	require.Equal(t, "web/web-1", m.selectedServiceRow().Key())
	assert.Equal(t, m.tr.CannotListProcessesStoppedInstance, mainText(m))

	openTab(m, p, "env")
	assert.NotEqual(t, m.tr.ServiceMultipleInstances, mainText(m))
}
