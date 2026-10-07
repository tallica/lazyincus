package commands

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ComposeBackup is one run of `incus-compose backup create`: a restore point
// on each of the stack's volumes, under one timestamp, which is also how
// every other backup verb names it.
type ComposeBackup struct {
	Timestamp string                `json:"timestamp"`
	Name      string                `json:"name"`
	Volumes   []ComposeBackupVolume `json:"volumes"`
	// Size is what the run's backup volumes take, which runs sharing a
	// volume each report in full; zero when incus-compose didn't say.
	Size int64 `json:"size"`
}

// ComposeBackupVolume pairs a stack's volume with the one holding its
// restore points.
type ComposeBackupVolume struct {
	Source ComposeBackupVolumeRef `json:"source"`
	Backup ComposeBackupVolumeRef `json:"backup"`
}

type ComposeBackupVolumeRef struct {
	Project string `json:"project"`
	Pool    string `json:"pool"`
	Name    string `json:"name"`
}

// CreatedAt is the timestamp as a time, zero for one that doesn't parse.
func (b *ComposeBackup) CreatedAt() time.Time {
	at, _ := time.Parse(time.RFC3339Nano, b.Timestamp)
	return at
}

// Pool is where the run's backup volumes are, the way `backup list` reports
// it: the first volume's.
func (b *ComposeBackup) Pool() string {
	if len(b.Volumes) == 0 {
		return ""
	}

	return b.Volumes[0].Backup.Pool
}

// BackupVerifyOK is `backup verify`'s one good status; the others are shown as given.
const BackupVerifyOK = "ok"

// BackupVolumeStatus is one volume's verdict from `backup verify`.
type BackupVolumeStatus struct {
	Volume string `json:"volume"`
	Status string `json:"status"`
}

// BackupVerification is `backup verify`'s report on one backup.
type BackupVerification struct {
	Timestamp string               `json:"timestamp"`
	Volumes   []BackupVolumeStatus `json:"volumes"`
}

// ComposeBackups lists the stack in dir's backups.
func (c *IncusCommand) ComposeBackups(dir string) ([]*ComposeBackup, error) {
	output, err := composeJSON(c.ComposeCmd(dir, "--ansi", "never", "backup", "list", "--format", "json"))
	if err != nil {
		return nil, err
	}

	return parseComposeBackups(output)
}

func parseComposeBackups(output []byte) ([]*ComposeBackup, error) {
	var backups []*ComposeBackup
	if err := json.Unmarshal(output, &backups); err != nil {
		return nil, err
	}

	return backups, nil
}

// VerifyComposeBackup checks the backup at timestamp. incus-compose exits
// non-zero when any volume isn't ok, having printed its report all the same,
// so a report is an answer whatever the exit status.
func (c *IncusCommand) VerifyComposeBackup(dir, timestamp string) (*BackupVerification, error) {
	output, err := composeJSON(c.ComposeCmd(dir, "--ansi", "never", "backup", "verify", "--format", "json", timestamp))
	if len(output) == 0 {
		return nil, err
	}

	var verification BackupVerification
	if jsonErr := json.Unmarshal(output, &verification); jsonErr != nil {
		return nil, errors.Join(err, jsonErr)
	}

	return &verification, nil
}

// composeJSON runs an incus-compose command printing JSON: stdout is the
// JSON alone, stderr its log lines, which on failure are the error.
func composeJSON(cmd *exec.Cmd) ([]byte, error) {
	output, err := cmd.Output()

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if message := strings.TrimSpace(string(exitErr.Stderr)); message != "" {
			return output, errors.New(message)
		}
	}

	if err != nil {
		return output, WrapError(err)
	}

	return output, nil
}
