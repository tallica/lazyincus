package gui

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
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

// withBackups lists backups for whichever stack is selected, recording
// which directories were asked about.
func withBackups(asked *[]string, mutex *sync.Mutex, backups []*commands.ComposeBackup, err error) func(*screen) {
	return func(s *screen) {
		s.gui.loadBackups = func(_ *commands.IncusCommand, dir string) ([]*commands.ComposeBackup, error) {
			mutex.Lock()
			*asked = append(*asked, dir)
			mutex.Unlock()

			return backups, err
		}
	}
}

func TestBackupsFollowTheStack(t *testing.T) {
	root := t.TempDir()
	local := testStack(t, root, "default", "web")

	var (
		asked []string
		mutex sync.Mutex
	)

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, local)(s)
		withBackups(&asked, &mutex, []*commands.ComposeBackup{
			testBackup("2026-09-20T15:27:46.833084Z", "", "db"),
			testBackup("2026-09-21T06:00:00Z", "nightly", "db", "cache"),
		}, nil)(s)
	})

	s.settle(t, "Services (default)")
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Backups) })

	screen := s.settle(t, "nightly")
	assert.Contains(t, screen, "Snapshots - Backups (default)")
	assert.Contains(t, screen, "2 vol")

	// Newest first.
	rows := onLoop(t, s, func() []string {
		return lo.Map(s.gui.Panels.Backups.List.GetItems(), func(backup *commands.ComposeBackup, _ int) string { return backup.Name })
	})
	assert.Equal(t, []string{"nightly", ""}, rows)

	mutex.Lock()
	defer mutex.Unlock()
	assert.Contains(t, asked, local.Dir)
}

// A new backup goes on top, pushing down the row the cursor was on; the
// cursor goes to the new one rather than following that row.
func TestNewBackupIsSelected(t *testing.T) {
	var (
		asked []string
		mutex sync.Mutex
	)

	old := testBackup("2026-09-20T15:27:46.833084Z", "", "db")
	backups := []*commands.ComposeBackup{old}

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)
		withBackups(&asked, &mutex, nil, nil)(s)
		s.gui.loadBackups = func(*commands.IncusCommand, string) ([]*commands.ComposeBackup, error) {
			mutex.Lock()
			defer mutex.Unlock()

			return backups, nil
		}
	})

	s.settle(t, "Services (default)")
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Backups) })
	s.settle(t, "1 vol")

	before := map[string]bool{old.Timestamp: true}

	mutex.Lock()
	backups = []*commands.ComposeBackup{testBackup("2026-10-07T21:59:00Z", "test", "db"), old}
	mutex.Unlock()

	assert.NoError(t, s.gui.refresh(func() error { return s.gui.selectNewBackup(before) }, s.gui.fetchBackups))

	assert.Eventually(t, func() bool {
		return onLoop(t, s, func() string {
			backup, err := s.gui.Panels.Backups.GetSelectedItem()
			if err != nil {
				return ""
			}

			return backup.Name
		}) == "test"
	}, 5*time.Second, 20*time.Millisecond)
}

func TestBackupsSayWhyTheyCantBeListed(t *testing.T) {
	var (
		asked []string
		mutex sync.Mutex
	)

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)
		withBackups(&asked, &mutex, nil, errors.New("ERR Getting the backup project\nmore"))(s)
	})

	s.settle(t, "Services (default)")
	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Backups) })

	s.settle(t, "Couldn't list backups: ERR Getting the backup project")
}

func TestBackupVolumesStr(t *testing.T) {
	gui := &Gui{}
	backup := testBackup("2026-09-21T06:00:00Z", "", "db", "cache")

	unverified := utils.Decolorise(gui.backupVolumesStr(backup, nil))
	assert.Contains(t, unverified, "vol-db    → default-backup/ic-backup-db")

	verified := utils.Decolorise(gui.backupVolumesStr(backup, &commands.BackupVerification{
		Volumes: []commands.BackupVolumeStatus{
			{Volume: "vol-cache", Status: "restore point missing"},
			{Volume: "vol-db", Status: "ok"},
			{Volume: "vol-new", Status: "not in this backup"},
		},
	}))
	assert.Contains(t, verified, "vol-cache → default-backup/ic-backup-cache restore point missing")
	assert.Contains(t, verified, "vol-db    → default-backup/ic-backup-db    ok")
	// A volume the stack has gained since, which the backup has nothing of.
	assert.Contains(t, verified, "vol-new                                    not in this backup")
}

// A long name gives way before a verification does.
func TestBackupRowKeepsItsVerification(t *testing.T) {
	var (
		asked []string
		mutex sync.Mutex
	)

	backup := testBackup("2026-09-21T06:00:00Z", "before-the-big-upgrade", "db", "cache")

	s := startScreenWith(t, 140, 40, nil, func(s *screen) {
		withStacks(t, testStack(t, t.TempDir(), "default", "web"))(s)
		withBackups(&asked, &mutex, []*commands.ComposeBackup{backup}, nil)(s)
	})

	s.settle(t, "Services (default)")
	s.do(t, func() error {
		s.gui.State.BackupVerifications = map[string]*commands.BackupVerification{
			s.gui.backupKey(backup): {Volumes: []commands.BackupVolumeStatus{{Status: commands.BackupVerifyOK}, {Status: commands.BackupVerifyOK}}},
		}

		if err := s.gui.switchFocus(s.gui.Views.Backups); err != nil {
			return err
		}

		return s.gui.Panels.Backups.RerenderList()
	})

	s.settle(t, "before-…")
	assert.Regexp(t, `before-.*2 vol\s+`+s.gui.Tr.BackupVerifiedOK, s.settle(t, "2 vol"))
}
