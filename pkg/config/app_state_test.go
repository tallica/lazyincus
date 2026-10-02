package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// Stacks saved before they had a remote are pinned to the one given, once:
// a second run finds nothing to write.
func TestPinningPlainStacks(t *testing.T) {
	dir := t.TempDir()
	appConfig := &AppConfig{ConfigDir: dir}

	require.NoError(t, appConfig.AddStack("/srv/web"))
	require.NoError(t, appConfig.AddStack("pve01:/srv/db"))
	require.NoError(t, appConfig.AddStack("web01:/srv/web"))
	require.NoError(t, appConfig.AddStack("/srv/db"))

	require.NoError(t, appConfig.PinStacks("web01"))

	state, err := appConfig.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"web01:/srv/web", "pve01:/srv/db", "web01:/srv/db"}, state.Stacks)

	info, err := os.Stat(appConfig.StateFilename())
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(appConfig.StateFilename(), info.ModTime(), info.ModTime().Add(-time.Hour)))

	require.NoError(t, appConfig.PinStacks("web01"))

	again, err := os.Stat(appConfig.StateFilename())
	require.NoError(t, err)
	assert.Equal(t, info.ModTime().Add(-time.Hour), again.ModTime())
}

func TestPinningNothingWritesNothing(t *testing.T) {
	appConfig := &AppConfig{ConfigDir: t.TempDir()}

	require.NoError(t, appConfig.PinStacks("web01"))
	assert.NoFileExists(t, appConfig.StateFilename())
}

// An edited stack keeps its place, and can't become one already listed.
func TestReplacingAStack(t *testing.T) {
	appConfig := &AppConfig{ConfigDir: t.TempDir()}

	for _, stack := range []string{"web01:/srv/web", "web01:/srv/db", "pve01:/srv/cache"} {
		require.NoError(t, appConfig.AddStack(stack))
	}

	require.NoError(t, appConfig.ReplaceStack("web01:/srv/db", "pve01:/srv/db"))
	require.ErrorIs(t, appConfig.ReplaceStack("web01:/srv/web", "pve01:/srv/cache"), ErrStackListed)

	state, err := appConfig.LoadAppState()
	require.NoError(t, err)
	assert.Equal(t, []string{"web01:/srv/web", "pve01:/srv/db", "pve01:/srv/cache"}, state.Stacks)
}
