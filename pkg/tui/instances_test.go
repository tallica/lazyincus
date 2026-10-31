package tui

import (
	"context"
	"os/exec"
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

func rowOf(m *Model, name string) string {
	for _, line := range screen(m) {
		if strings.Contains(line, "│"+name+" ") {
			return line
		}
	}

	return ""
}

func TestStartingMarksTheRowUntilTheListingLands(t *testing.T) {
	m := start(t, options{}, 90, 16)
	press(m, "j", "j")
	require.Equal(t, "db", m.selectedInstance().Name)

	_, cmd := m.Update(keyPress("S"))
	assert.Contains(t, rowOf(m, "db"), "starting")
	assert.Regexp(t, `^starting `, footer(m))

	run(m, cmd)
	assert.Contains(t, rowOf(m, "db"), "running")
	assert.NotContains(t, footer(m), "starting")
}

func TestStoppingAsksFirstNamingTheInstance(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "s")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to stop instance cache?", ansi.Strip(m.popups[0].message))

	press(m, "y")
	assert.Contains(t, rowOf(m, "cache"), "stopped")
}

func TestAFailedActionTakesItsMarkOffAndSaysWhy(t *testing.T) {
	server := instancesServer(nil, runningInstance("web", nil))
	m := start(t, options{server: server}, 90, 16)
	server.SetInstances(nil)

	press(m, "r")
	assert.NotContains(t, rowOf(m, "web"), "restarting")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "Instance not found")
}

func TestLifecycleKeysAreRefusedReadOnly(t *testing.T) {
	m := start(t, options{readOnly: true}, 90, 16)

	for _, key := range []string{"S", "s", "r", "p", "d"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}

func rename(m *Model, to string) {
	run(m, onInstance((*Model).instanceRename)(m))
	press(m, "ctrl+u")
	typeText(m, to)
	press(m, "enter")
}

func TestRenamingAStoppedInstanceFollowsItToItsNewName(t *testing.T) {
	m := start(t, options{}, 90, 16)
	press(m, "j", "j")

	rename(m, "archive")
	assert.Equal(t, "archive", m.selectedInstance().Name)
	assert.Equal(t, []string{"default/cache", "default/web", "default/archive"}, shownInstances(m))
}

func TestRenamingARunningInstanceStopsItOnceConfirmed(t *testing.T) {
	m := start(t, options{}, 90, 16)

	rename(m, "redis")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "Stop cache, rename it to redis and start it again?")

	press(m, "y")
	assert.Equal(t, "redis", m.selectedInstance().Name)
	assert.Contains(t, rowOf(m, "redis"), "running")
}

func TestTheDescriptionIsSavedFromItsPrompt(t *testing.T) {
	m := start(t, options{}, 90, 16)

	run(m, onInstance((*Model).instanceEditDescription)(m))
	typeText(m, "the cache")
	press(m, "enter")
	assert.Equal(t, "the cache", m.selectedInstance().Instance.Description)
}

func TestACopyLandsAsATransientStatus(t *testing.T) {
	m := start(t, options{}, 90, 16)

	press(m, "y")
	require.NotNil(t, m.topMenu())
	assert.Contains(t, strings.Join(screen(m), "\n"), "cache")

	press(m, "esc")
	m.Update(copiedMsg{value: "cache"})
	assert.Regexp(t, `^copied to clipboard: cache `, footer(m))
}

func TestShellAndConsoleRefuseAStoppedInstance(t *testing.T) {
	m := start(t, options{}, 90, 16)
	press(m, "j", "j")

	for key, message := range map[string]string{
		"E": m.tr.CannotExecStoppedInstanceError, "a": m.tr.CannotAttachStoppedInstanceError,
	} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Equal(t, message, ansi.Strip(m.popups[0].message), key)
		press(m, "esc")
	}
}

func TestASubprocessIsEchoedAndWaitsForEnter(t *testing.T) {
	m := newTestModel(t, options{})
	var out strings.Builder

	s := &subprocess{
		cmd: exec.Command("echo", "hi"), message: "bye", os: m.os, log: m.incus.Log, pressEnter: m.tr.PressEnterToReturn,
	}
	s.SetStdin(strings.NewReader("\n"))
	s.SetStdout(&out)
	require.NoError(t, s.Run())

	text := ansi.Strip(out.String())
	assert.Contains(t, text, "+ echo hi\n")
	assert.Contains(t, text, "bye")
	assert.Contains(t, text, "hi\n")
	assert.True(t, strings.HasSuffix(text, m.tr.PressEnterToReturn))
}
