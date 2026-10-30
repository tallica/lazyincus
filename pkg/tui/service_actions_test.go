package tui

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replicaModel lists the stack default's service web with two replicas, recording what runs on the terminal.
func replicaModel(t *testing.T) (*Model, *[]*exec.Cmd) {
	t.Helper()

	m := replicatedStack(t, composeFixture("default", "web-1", "web"), composeFixture("default", "web-2", "web"))

	var ran []*exec.Cmd

	m.runOnTerminal = func(cmd *exec.Cmd, then func() tea.Cmd) tea.Cmd {
		ran = append(ran, cmd)
		return then()
	}

	press(m, "2")
	require.Equal(t, "web", m.selectedServiceRow().Key())

	return m, &ran
}

func optionKeys(m *Model) string {
	return strings.Join(lo.FilterMap(m.optionsBindings("services"), func(b *binding, _ int) (string, bool) {
		return b.key, hasRune(b.key)
	}), "")
}

func TestServiceRowKeysAreComposeVerbs(t *testing.T) {
	m, ran := replicaModel(t)

	press(m, "r")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"restart", "web"}, (*ran)[0].Args[len((*ran)[0].Args)-2:])

	press(m, "s")
	require.Len(t, m.popups, 1)
	assert.Equal(t, "Are you sure you want to stop service web?", ansi.Strip(m.popups[0].message))
	press(m, "y")
	assert.Equal(t, "stop", (*ran)[1].Args[len((*ran)[1].Args)-2])

	press(m, "d")
	require.NotNil(t, m.topMenu())
	assert.Equal(t, m.tr.ComposeDownMenuTitle, m.topMenu().title)
}

func TestReplicaRowKeysAreTheInstances(t *testing.T) {
	m, ran := replicaModel(t)

	press(m, "j", "s")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "stop instance web-1")
	press(m, "n")

	press(m, "d")
	require.Len(t, m.popups, 1)
	assert.Contains(t, ansi.Strip(m.popups[0].message), "web-1")
	assert.Contains(t, ansi.Strip(m.popups[0].message), "delete")
	assert.Empty(t, *ran)
}

func TestNewSnapshotOnAServiceRowAsksWhichReplica(t *testing.T) {
	m, _ := replicaModel(t)

	press(m, "n")
	require.NotNil(t, m.topMenu())
	assert.ElementsMatch(t, []string{"web-1", "web-2"}, lo.Compact(lo.Map(m.topMenu().items, func(i *menuItem, _ int) string { return i.label })))
	press(m, "esc", "j", "n")
	assert.Nil(t, m.topMenu(), "a replica's row goes straight to the prompt")
	assert.NotEmpty(t, m.popups)
}

func TestServiceOptionsFollowTheRow(t *testing.T) {
	m, _ := replicaModel(t)

	assert.True(t, strings.HasPrefix(optionKeys(m), "udUSsrpfbgmMnEy"), optionKeys(m))

	press(m, "j")
	assert.True(t, strings.HasPrefix(optionKeys(m), "SsrpdfnmMyEuUbg"), optionKeys(m))

	descriptions := lo.Map(m.optionsBindings("services"), func(b *binding, _ int) string { return b.key + " " + b.description })
	assert.Contains(t, descriptions, "d "+m.tr.Remove)
	assert.Contains(t, descriptions, "u bring up service")
}

func TestStoppingAReplicaRollsUpOnItsStack(t *testing.T) {
	m, _ := replicaModel(t)

	press(m, "j", "s", "y")
	assert.Contains(t, strings.Join(rowsOf(m, "stacks"), "\n"), "partial")
}
