package tui

import (
	"errors"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
)

func testBackup(timestamp, name string, volumes ...string) *commands.ComposeBackup {
	backup := &commands.ComposeBackup{Timestamp: timestamp, Name: name}
	for _, volume := range volumes {
		backup.Volumes = append(backup.Volumes, commands.ComposeBackupVolume{
			Source: commands.ComposeBackupVolumeRef{Project: "default", Pool: "default", Name: "vol-" + volume},
			Backup: commands.ComposeBackupVolumeRef{Project: "default-backup", Pool: "default", Name: "ic-backup-" + volume},
		})
	}

	return backup
}

func backupNames(m *Model) []string {
	names := make([]string, len(m.backups))
	for i, backup := range m.backups {
		names[i] = backup.Name
	}

	return names
}

// backupsModel is composeModel with the stack's backups read from *backups, or failing with err.
func backupsModel(t *testing.T, backups *[]*commands.ComposeBackup, err *error) (*Model, *[]*exec.Cmd) {
	t.Helper()

	m, ran, _ := composeModel(t, options{})
	m.loadBackups = func(*commands.IncusCommand, string) ([]*commands.ComposeBackup, error) { return *backups, *err }
	run(m, refresh(m.fetchBackups()))

	return m, ran
}

func TestBackupsFollowTheStack(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{
		testBackup("2026-09-20T15:27:46.833084Z", "", "db"),
		testBackup("2026-09-21T06:00:00Z", "nightly", "db", "cache"),
	}
	m, _ := backupsModel(t, &backups, &failure)

	assert.Equal(t, "Backups (default)", m.backupsTitle())
	assert.Equal(t, []string{"nightly", ""}, backupNames(m))

	press(m, "B")
	assert.Equal(t, "backups", m.currentView())
	assert.Contains(t, rowsOf(m, "backups")[0], "2 vol")
}

func TestBackupsSayWhyTheyCantBeListed(t *testing.T) {
	var backups []*commands.ComposeBackup

	failure := errors.New("ERR Getting the backup project\nmore")
	m, _ := backupsModel(t, &backups, &failure)

	assert.Equal(t, "Couldn't list backups: ERR Getting the backup project", m.panelNamed("backups").empty)
	assert.Empty(t, m.popups)
}

// A new backup goes on top, pushing down the row the cursor was on; the
// cursor goes to the new one rather than following that row.
func TestNewBackupIsSelected(t *testing.T) {
	var failure error

	old := testBackup("2026-09-20T15:27:46.833084Z", "", "db")
	backups := []*commands.ComposeBackup{old}
	m, ran := backupsModel(t, &backups, &failure)

	run := m.runOnTerminal
	m.runOnTerminal = func(cmd *exec.Cmd, then func() tea.Cmd) tea.Cmd {
		backups = []*commands.ComposeBackup{testBackup("2026-10-07T21:59:00Z", "test", "db"), old}

		return run(cmd, then)
	}

	press(m, "B", "n", "enter", "enter")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"backup", "create"}, (*ran)[0].Args[len((*ran)[0].Args)-2:])
	assert.Equal(t, []string{"test", ""}, backupNames(m))
	assert.Equal(t, "test", m.selectedBackup().Name)
}

func TestNamedLiveBackupArguments(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{}
	m, ran := backupsModel(t, &backups, &failure)

	press(m, "B", "n")
	m.popups[len(m.popups)-1].input.SetValue("nightly")
	press(m, "enter", "j", "enter")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"backup", "create", "--name", "nightly", "--live"}, (*ran)[0].Args[len((*ran)[0].Args)-5:])
}

func TestPruningCountsWhatItDeletes(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{testBackup("2026-09-20T15:27:46Z", "a", "db"), testBackup("2026-09-21T06:00:00Z", "b", "db")}
	m, ran := backupsModel(t, &backups, &failure)

	press(m, "B", "D")
	m.popups[len(m.popups)-1].input.SetValue("x")
	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, `"x"`)
	press(m, "esc", "D")

	m.popups[len(m.popups)-1].input.SetValue("1")
	press(m, "enter")
	require.Len(t, m.popups, 1)
	assert.Contains(t, m.popups[0].message, "1")
	assert.Empty(t, *ran)

	press(m, "y")
	require.Len(t, *ran, 1)
	assert.Equal(t, []string{"backup", "delete", "--keep-last", "1"}, (*ran)[0].Args[len((*ran)[0].Args)-4:])
}

// The verification stays on its row across a refresh.
func TestBackupRowKeepsItsVerification(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{testBackup("2026-09-21T06:00:00Z", "nightly", "db", "cache")}
	m, _ := backupsModel(t, &backups, &failure)

	m.backupVerifications = map[string]*commands.BackupVerification{
		m.backupKey(m.backups[0]): {Volumes: []commands.BackupVolumeStatus{{Status: commands.BackupVerifyOK}, {Status: commands.BackupVerifyOK}}},
	}
	run(m, refresh(m.fetchBackups()))
	press(m, "B")

	assert.Contains(t, rowsOf(m, "backups")[0], m.tr.BackupVerifiedOK)
	assert.Contains(t, mainText(m), "vol-db")
}

// A restore can be narrowed to the services with a named volume.
func TestRestoreOffersTheWholeStack(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{testBackup("2026-09-21T06:00:00Z", "nightly", "db")}
	m, ran := backupsModel(t, &backups, &failure)

	press(m, "B", "r", "enter")
	require.Len(t, *ran, 1)
	args := (*ran)[0].Args
	assert.Equal(t, []string{"backup", "restore", "2026-09-21T06:00:00Z"}, args[len(args)-3:])
}

// B goes to the Backups tab even when Snapshots was the last of the two on show.
func TestBGoesToTheStacksBackups(t *testing.T) {
	var failure error

	backups := []*commands.ComposeBackup{testBackup("2026-09-21T06:00:00Z", "nightly", "db")}
	m, _ := backupsModel(t, &backups, &failure)

	for _, from := range []string{"1", "2"} {
		press(m, "4", from, "B")
		assert.Equal(t, "backups", m.currentView(), "from panel %s", from)
	}
}
