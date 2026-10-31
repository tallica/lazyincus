package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/config"
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

// A reload swaps UserConfig on the loop; o and y run off it, so they take their command first.
func TestOpenAndCopyUseTheCommandOfTheirKeypress(t *testing.T) {
	dir := t.TempDir()
	before, after := filepath.Join(dir, "before"), filepath.Join(dir, "after")
	m := start(t, options{configure: func(c *config.UserConfig) {
		c.OS.OpenCommand, c.OS.CopyToClipboardCommand = "before {{filename}}", "tee "+before
	}}, 140, 40)
	var opened string
	m.openFile = func(command, _ string) error {
		opened = command
		return nil
	}

	open, copyValue := m.openConfig(), m.copyToClipboard("web")
	reloaded := *m.cfg.UserConfig
	reloaded.OS.OpenCommand, reloaded.OS.CopyToClipboardCommand = "after {{filename}}", "tee "+after
	m.cfg.UserConfig = &reloaded

	open()
	assert.Equal(t, "before {{filename}}", opened)

	require.NoError(t, copyValue().(copiedMsg).err)
	copied, err := os.ReadFile(before)
	require.NoError(t, err)
	assert.Equal(t, "web", string(copied))
	assert.NoFileExists(t, after)
}
