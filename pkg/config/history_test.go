package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A line run again moves to the newest rather than being kept twice, each
// history is its own, and the oldest go past historyLength.
func TestRecordingCommands(t *testing.T) {
	dir := t.TempDir()
	appConfig := &AppConfig{ConfigDir: dir}

	require.NoError(t, appConfig.RecordCommand(IncusHistory, "list"))
	require.NoError(t, appConfig.RecordCommand(IncusHistory, "info web"))
	require.NoError(t, appConfig.RecordCommand(IncusHistory, "list"))
	require.NoError(t, appConfig.RecordCommand(ComposeHistory, "ps"))

	lines, err := appConfig.LoadHistory(IncusHistory)
	require.NoError(t, err)
	assert.Equal(t, []string{"info web", "list"}, lines)

	lines, err = appConfig.LoadHistory(ComposeHistory)
	require.NoError(t, err)
	assert.Equal(t, []string{"ps"}, lines)

	for i := range historyLength {
		require.NoError(t, appConfig.RecordCommand(IncusHistory, fmt.Sprintf("info %d", i)))
	}

	lines, err = appConfig.LoadHistory(IncusHistory)
	require.NoError(t, err)
	assert.Len(t, lines, historyLength)
	assert.Equal(t, "info 0", lines[0])

	// Its own file, and only its owner's to read: a line holds whatever was
	// typed on it.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "history.yml", entries[0].Name())

	info, err := os.Stat(appConfig.HistoryFilename())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestMissingHistoryIsEmpty(t *testing.T) {
	lines, err := (&AppConfig{ConfigDir: t.TempDir()}).LoadHistory(IncusHistory)
	require.NoError(t, err)
	assert.Empty(t, lines)
}
