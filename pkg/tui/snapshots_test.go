package tui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/utils"
)

func snapshotRows(m *Model) []string {
	p := m.panelNamed("snapshots")
	rows := make([]string, len(p.rows))
	for i, row := range p.rows {
		rows[i] = strings.Join(strings.Fields(utils.Decolorise(row)), " ")
	}

	return rows
}

func TestSnapshotsFollowTheSelectedInstanceNewestFirst(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	assert.Empty(t, snapshotRows(m), "a-name-long-enough-to-be-cut-off has none")
	press(m, "j")
	require.Equal(t, "web", m.selectedInstance().Name)
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())

	rows := snapshotRows(m)
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "daily-b "), rows[0])
	assert.True(t, strings.HasPrefix(rows[1], "daily-a "), rows[1])
}

func TestSnapshotsKeepFollowingWhileTheirOwnListHasFocus(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "j", "2", "j")
	assert.Equal(t, "snapshots", m.currentView())
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())
	assert.Equal(t, "daily-a", m.selectedSnapshot().Name)

	refreshNow(m)
	assert.Equal(t, "daily-a", m.selectedSnapshot().Name, "the selection follows its snapshot across a refresh")
}

func TestESnapshotsListsEveryInstancesWithItsOwner(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "2", "e")
	assert.Equal(t, "Snapshots (all)", m.snapshotsTitle())
	rows := snapshotRows(m)
	require.Len(t, rows, 2)
	assert.True(t, strings.HasPrefix(rows[0], "daily-b web "), rows[0])

	press(m, "e")
	assert.Equal(t, "Snapshots (a-name-long-enough-to-be-cut-off)", m.snapshotsTitle(), "back to Instances' selection")
	assert.Empty(t, snapshotRows(m))
}

func TestTheSnapshotConfigTabNamesItsInstance(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)

	press(m, "j", "2")
	text := mainText(m)
	assert.Contains(t, text, "Instance:     web")
	assert.Contains(t, text, "Name:         daily-b")
	assert.Contains(t, text, "Full details:")
}

// onWebsNewest has Snapshots focused on web's daily-b.
func onWebsNewest(t *testing.T, opts options) *Model {
	t.Helper()

	opts.fixture = opts.server == nil
	m := start(t, opts, 140, 40)
	press(m, "j", "2")
	require.Equal(t, "daily-b", m.selectedSnapshot().Name)

	return m
}

func TestRestoringASnapshotAsksThenMarksItsInstance(t *testing.T) {
	m := onWebsNewest(t, options{})

	press(m, "r")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to restore web to snapshot daily-b? Anything changed since is lost.",
		ansi.Strip(m.popups[0].message))

	_, cmd := m.Update(keyPress("y"))
	assert.Contains(t, rowOf(m, "web"), "restoring")
	run(m, cmd)
	assert.Contains(t, rowOf(m, "web"), "running")
}

func TestDeletingASnapshotNamesItsInstance(t *testing.T) {
	m := onWebsNewest(t, options{})

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to delete snapshot daily-b of web?", ansi.Strip(m.popups[0].message))
}

func TestSnapshotKeysAreRefusedReadOnly(t *testing.T) {
	m := onWebsNewest(t, options{readOnly: true})

	for _, key := range []string{"r", "d"} {
		press(m, key)
		require.Len(t, m.popups, 1, key)
		assert.Contains(t, m.popups[0].message, "--read-only", key)
		press(m, "esc")
	}
}

func TestRenamingASnapshotFollowsIt(t *testing.T) {
	m := onWebsNewest(t, options{})

	run(m, onSnapshot((*Model).snapshotRename)(m))
	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "Rename snapshot daily-b of web", m.topPrompt().title)
	press(m, "ctrl+u")
	typeText(m, "weekly")
	press(m, "enter")

	assert.Equal(t, "weekly", m.selectedSnapshot().Name)
}

func TestANewInstanceFromASnapshotIsCopiedAndFocused(t *testing.T) {
	server := incustest.Fixture()
	m := onWebsNewest(t, options{server: server})

	var ran []string
	m.runIncus = func(cmd *exec.Cmd) (string, error) {
		copied := incustest.Fixture().Instances[0]
		copied.Name, copied.Status, copied.Snapshots = "web-daily-b", "Stopped", nil
		server.SetInstances(append(incustest.Fixture().Instances, copied))
		ran = cmd.Args

		return "", nil
	}

	run(m, onSnapshot((*Model).snapshotNewInstance)(m))
	assert.Equal(t, "New instance from web/daily-b", m.topPrompt().title)
	press(m, "enter")

	assert.Equal(t, []string{"incus", "--project", "default", "copy", "web/daily-b", "web-daily-b"}, ran)
	assert.Equal(t, "instances", m.currentView())
	assert.Equal(t, "web-daily-b", m.selectedInstance().Name)
	assert.Equal(t, "Snapshots (web-daily-b)", m.snapshotsTitle())
}

func TestANewInstanceFromASnapshotSaysWhyItFailed(t *testing.T) {
	m := onWebsNewest(t, options{})
	m.runIncus = func(*exec.Cmd) (string, error) {
		return "", errors.New(`Error: Instance name "web-daily-b" already in use`)
	}

	run(m, onSnapshot((*Model).snapshotNewInstance)(m))
	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "already in use")
}

// A compose instance's copy would keep its service label and join the service.
func TestANewInstanceFromAComposeSnapshotIsRefused(t *testing.T) {
	api1 := incustest.Fixture().Instances[0]
	api1.Name = "api-1"
	api1.ExpandedConfig = map[string]string{"user.label.incus-compose.service": "api"}
	api1.Snapshots = []api.InstanceSnapshot{{Name: "api-1/nightly"}}
	m := start(t, options{server: incustest.New(incustest.Server{Instances: []api.InstanceFull{api1}})}, 140, 40)
	press(m, "2")

	run(m, onSnapshot((*Model).snapshotNewInstance)(m))
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "api-1 belongs to a compose stack")
}

func TestYOffersASnapshotsNameAndReference(t *testing.T) {
	m := onWebsNewest(t, options{})

	press(m, "y")
	require.NotNil(t, m.topMenu())
	assert.Contains(t, strings.Join(screen(m), "\n"), "web/daily-b")
}

func TestNOnInstancesTakesASnapshotAndLandsOnIt(t *testing.T) {
	m := start(t, options{fixture: true}, 140, 40)
	press(m, "j")

	press(m, "n")
	require.NotNil(t, m.topPrompt())
	assert.Equal(t, "New snapshot of web", m.topPrompt().title)
	typeText(m, "manual")
	press(m, "enter")

	assert.Equal(t, "snapshots", m.currentView())
	assert.Equal(t, "manual", m.selectedSnapshot().Name)
	assert.Equal(t, "Snapshots (web)", m.snapshotsTitle())
}

func TestTheSnapshotOptionsAreFieldsTabReaches(t *testing.T) {
	m := onWebsNewest(t, options{})

	press(m, "n", "tab")
	assert.Equal(t, snapshotOptionsView, m.currentView())
	assert.Contains(t, footer(m), "← →: change value")
	assert.Contains(t, strings.Join(screen(m), "\n"), "<enter> or <ctrl+s> to create")

	press(m, "l", "j", "space")
	opts := m.popups[0].snapshot.options()
	assert.True(t, opts.NoExpiry)
	assert.True(t, opts.Stateful)
	assert.Contains(t, strings.Join(screen(m), "\n"), "Stateful    ‹ yes ›")

	press(m, "enter")
	assert.Equal(t, confirmationView, m.currentView(), "no name yet: back to it")
	press(m, "q")
	assert.Equal(t, "q", m.topPrompt().input.Value(), "typed in the name, not closing")

	press(m, "tab", "q")
	assert.Empty(t, m.popups, "q closes from the options")
}

func TestNIsRefusedReadOnly(t *testing.T) {
	m := onWebsNewest(t, options{readOnly: true})

	press(m, "n")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "--read-only")
}
