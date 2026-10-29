package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func instancesServer(consoleLogs map[string]string, instances ...api.InstanceFull) *incustest.Server {
	return incustest.New(incustest.Server{Version: "7.4", Instances: instances, ConsoleLogs: consoleLogs})
}

func runningInstance(name string, config map[string]string) api.InstanceFull {
	return api.InstanceFull{Instance: api.Instance{
		Name: name, Project: "default", Status: "Running", Type: "container", ExpandedConfig: config,
	}}
}

func TestTheLogsTabShowsTheConsoleLogFollowingItsEnd(t *testing.T) {
	server := instancesServer(map[string]string{"web": "booted\n"}, runningInstance("web", nil))
	m := start(t, options{server: server}, 90, 16)

	press(m, "]")
	assert.Contains(t, mainText(m), "booted")
	assert.True(t, m.main.autoscroll)
}

func TestMOpensTheLogsTabInMain(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "m")
	assert.Equal(t, mainView, m.currentView())
	assert.Equal(t, 1, m.panelNamed("instances").mainTab)
}

func TestTheEnvTabListsTheEnvironmentKeys(t *testing.T) {
	config := map[string]string{"environment.TZ": "UTC", "environment.APP": "web", "limits.cpu": "2"}
	server := instancesServer(nil, runningInstance("web", config), runningInstance("bare", nil))
	m := start(t, options{server: server}, 90, 16)

	press(m, "]", "]", "]")
	assert.Equal(t, "Nothing to display", mainText(m), "bare sorts first")
	press(m, "j")
	assert.Equal(t, "APP=web\nTZ=UTC", mainText(m))
}

func TestTheTopTabOfAStoppedInstanceSaysWhyItIsEmpty(t *testing.T) {
	m := start(t, options{}, 90, 16)

	// Not a running one: the fake daemon can't exec.
	press(m, "j", "j")
	require.Equal(t, "db", m.selectedInstance().Name)
	press(m, "[")
	assert.Equal(t, m.tr.CannotListProcessesStoppedInstance, mainText(m))
}

func TestATickerTaskRedrawsInPlaceUntilItStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan tea.Msg, 16)
	w := &mainWriter{send: func(msg tea.Msg) { sent <- msg }}

	ticks := 0
	done := make(chan struct{})
	go func() {
		tickerTask(time.Millisecond, func() string {
			if ticks++; ticks == 3 {
				cancel()
			}
			return "tick"
		})(ctx, w)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the ticker never stopped")
	}

	require.Len(t, sent, 4)
	assert.Equal(t, writeClear, (<-sent).(mainWriteMsg).kind)
	for range 3 {
		assert.Equal(t, writeRerender, (<-sent).(mainWriteMsg).kind)
	}
}

func instanceIn(project, name, status string) api.InstanceFull {
	return api.InstanceFull{Instance: api.Instance{Name: name, Project: project, Status: status, Type: "container"}}
}

func shownInstances(m *Model) []string {
	names := make([]string, 0, m.panelNamed("instances").count())
	for _, item := range m.panelNamed("instances").shown {
		names = append(names, m.instances[item].Project+"/"+m.instances[item].Name)
	}

	return names
}

func TestInstancesSortByProjectThenStoppedLast(t *testing.T) {
	server := instancesServer(nil,
		instanceIn("shop", "api", "Stopped"), instanceIn("shop", "web", "Running"),
		instanceIn("default", "db", "Stopped"), instanceIn("default", "cache", "Running"))
	m := start(t, options{server: server}, 90, 16)

	assert.Equal(t, []string{"default/cache", "default/db", "shop/web", "shop/api"}, shownInstances(m))
	assert.True(t, m.showProject)
}

func TestEHidesStoppedInstancesAndBringsThemBack(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "e")
	assert.Equal(t, []string{"default/cache", "default/web"}, shownInstances(m))
	assert.False(t, m.showProject)

	press(m, "e")
	assert.Equal(t, []string{"default/cache", "default/web", "default/db"}, shownInstances(m))
}
