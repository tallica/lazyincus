package tui

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOOpensTheConfigFile(t *testing.T) {
	m := start(t, options{}, 140, 40)
	var opened []string
	m.openFile = func(command, name string) error {
		opened = append(opened, command, name)
		return nil
	}

	press(m, "o")

	assert.Equal(t, []string{m.cfg.UserConfig.OS.OpenCommand, m.cfg.ConfigFilename()}, opened)
	assert.Empty(t, m.popups)
}

func TestOFailingToOpenIsAnError(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openFile = func(string, string) error { return errors.New("no opener") }

	press(m, "o")

	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.ErrorTitle, m.popups[0].title)
	assert.Contains(t, m.popups[0].message, "no opener")
}

func TestOverAMenuOGoesToTheMenu(t *testing.T) {
	m := start(t, options{}, 140, 40)
	m.openFile = func(string, string) error {
		t.Error("the config was opened")
		return nil
	}

	press(m, "x", "o")

	assert.Equal(t, menuView, m.popups[0].view)
}

func TestOEditsTheConfigAndAppliesIt(t *testing.T) {
	t.Setenv("VISUAL", "myeditor")
	m := start(t, options{}, 140, 40)
	before := m.theme.activeBorder
	var ran []*exec.Cmd
	m.runOnTerminal = func(cmd *exec.Cmd, after func(error), then func(error) tea.Cmd) tea.Cmd {
		ran = append(ran, cmd)
		require.NoError(t, os.WriteFile(m.cfg.ConfigFilename(), []byte("gui:\n  theme:\n    activeBorderColor: [red]\n"), 0o600))

		return then(nil)
	}

	press(m, "O")

	require.Len(t, ran, 1)
	assert.Equal(t, []string{"myeditor", m.cfg.ConfigFilename()}, ran[0].Args)
	assert.NotEqual(t, before, m.theme.activeBorder)
	assert.Empty(t, m.popups)
}

func TestOWithoutAnEditorIsAnError(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", "")
	m := start(t, options{}, 140, 40)

	press(m, "O")

	require.Len(t, m.popups, 1)
	assert.Equal(t, m.tr.ErrorTitle, m.popups[0].title)
}
