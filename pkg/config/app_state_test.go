package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingStateIsEmpty(t *testing.T) {
	appConfig := &AppConfig{ConfigDir: t.TempDir()}

	state, err := appConfig.LoadAppState()
	require.NoError(t, err)
	assert.Empty(t, state.Stacks)
}

func TestStacksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	appConfig := &AppConfig{ConfigDir: dir}

	require.NoError(t, appConfig.AddStack("/srv/web"))
	require.NoError(t, appConfig.AddStack("/srv/db"))
	require.ErrorIs(t, appConfig.AddStack("/srv/web"), ErrStackListed)

	state, err := appConfig.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"/srv/web", "/srv/db"}, state.Stacks)

	require.NoError(t, appConfig.RemoveStack("/srv/web"))
	require.NoError(t, appConfig.RemoveStack("/srv/never-added"))

	state, err = appConfig.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"/srv/db"}, state.Stacks)

	// Only the state file: the rename leaves no temp file behind, and the
	// config is never written.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state.yml", entries[0].Name())
}

// A second session's add lands between this one's read and its write: the
// write re-reads first, so neither is lost.
func TestAddingKeepsAnotherSessionsStacks(t *testing.T) {
	dir := t.TempDir()
	one := &AppConfig{ConfigDir: dir}
	other := &AppConfig{ConfigDir: dir}

	_, err := one.LoadAppState()
	require.NoError(t, err)

	require.NoError(t, other.AddStack("/srv/other"))
	require.NoError(t, one.AddStack("/srv/one"))

	state, err := one.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"/srv/other", "/srv/one"}, state.Stacks)
}

func TestUnreadableStateIsAnError(t *testing.T) {
	dir := t.TempDir()
	appConfig := &AppConfig{ConfigDir: dir}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.yml"), []byte("stacks: [unclosed\n"), 0o600))

	_, err := appConfig.LoadAppState()
	require.Error(t, err)

	// And an add doesn't paper over it by writing a fresh file.
	require.Error(t, appConfig.AddStack("/srv/web"))
}
