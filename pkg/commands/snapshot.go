package commands

import (
	"strings"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Snapshot is one snapshot of an instance. Snapshots have no identity of
// their own in Incus: every operation names the instance and the snapshot.
type Snapshot struct {
	InstanceName string
	Name         string

	Snapshot  api.InstanceSnapshot
	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

func (s *Snapshot) Key() string {
	return s.InstanceName + "/" + s.Name
}

func (s *Snapshot) Delete() error {
	op, err := s.Client.DeleteInstanceSnapshot(s.InstanceName, s.Name)
	if err != nil {
		return err
	}

	return op.Wait()
}

// Restore rolls the instance back to this snapshot. Incus takes a restore as
// an update to the instance itself, so this reads the instance first to keep
// the rest of its config intact.
func (s *Snapshot) Restore() error {
	instance, etag, err := s.Client.GetInstance(s.InstanceName)
	if err != nil {
		return err
	}

	instance.Restore = s.Name

	op, err := s.Client.UpdateInstance(s.InstanceName, instance.Writable(), etag)
	if err != nil {
		return err
	}

	return op.Wait()
}

// snapshotName strips the instance prefix Incus puts on snapshot names in
// some responses, leaving the bare name every other call expects.
func snapshotName(name string) string {
	if index := strings.LastIndex(name, "/"); index >= 0 {
		return name[index+1:]
	}

	return name
}

// Snapshots lists the instance's snapshots.
func (i *Instance) Snapshots() ([]*Snapshot, error) {
	apiSnapshots, err := i.Client.GetInstanceSnapshots(i.Name)
	if err != nil {
		return nil, err
	}

	snapshots := make([]*Snapshot, len(apiSnapshots))

	for index := range apiSnapshots {
		apiSnapshot := apiSnapshots[index]

		snapshots[index] = &Snapshot{
			// GetInstanceSnapshots returns names as "<instance>/<snapshot>";
			// every other call wants the bare snapshot name.
			InstanceName: i.Name,
			Name:         snapshotName(apiSnapshot.Name),
			Snapshot:     apiSnapshot,
			Client:       i.Client,
			OSCommand:    i.OSCommand,
			Log:          i.Log,
			Tr:           i.Tr,
		}
	}

	return snapshots, nil
}

// SnapshotOptions are the choices `incus snapshot create` exposes beyond the
// name.
type SnapshotOptions struct {
	// Stateful includes the instance's runtime state, which needs CRIU on
	// the host and a running instance. Incus rejects it otherwise.
	Stateful bool

	// ExpiresIn deletes the snapshot after this long. Zero keeps it until
	// something deletes it.
	ExpiresIn time.Duration
}

// CreateSnapshot takes a snapshot of the instance.
func (i *Instance) CreateSnapshot(name string, opts SnapshotOptions) error {
	post := api.InstanceSnapshotsPost{
		Name:     name,
		Stateful: opts.Stateful,
	}

	if opts.ExpiresIn > 0 {
		expiresAt := time.Now().Add(opts.ExpiresIn)
		post.ExpiresAt = &expiresAt
	}

	op, err := i.Client.CreateInstanceSnapshot(i.Name, post)
	if err != nil {
		return err
	}

	return op.Wait()
}
