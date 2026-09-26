package commands

import (
	"strings"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Snapshot is one snapshot of an instance, or of a custom volume. Snapshots
// have no identity of their own in Incus: every operation names what they
// were taken of and the snapshot.
type Snapshot struct {
	Project string
	// Owner is the name of the instance the snapshot was taken of, or of
	// the volume.
	Owner string
	Name  string

	// Volume is the volume a volume snapshot was taken of, nil for an
	// instance's; VolumeSnapshot is then what the daemon says of it, and
	// Snapshot otherwise.
	Volume         *Volume
	Snapshot       api.InstanceSnapshot
	VolumeSnapshot api.StorageVolumeSnapshot

	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

func (s *Snapshot) Key() string {
	if s.Volume != nil {
		return "volume/" + s.Volume.Key() + "/" + s.Name
	}

	return s.Project + "/" + s.Owner + "/" + s.Name
}

func (s *Snapshot) CreatedAt() time.Time {
	if s.Volume != nil {
		return s.VolumeSnapshot.CreatedAt
	}

	return s.Snapshot.CreatedAt
}

// ExpiresAt is zero for a snapshot kept until something deletes it.
func (s *Snapshot) ExpiresAt() time.Time {
	if s.Volume != nil {
		if s.VolumeSnapshot.ExpiresAt == nil {
			return time.Time{}
		}

		return *s.VolumeSnapshot.ExpiresAt
	}

	return s.Snapshot.ExpiresAt
}

// IsStateful is never true of a volume, which has no runtime state.
func (s *Snapshot) IsStateful() bool {
	return s.Volume == nil && s.Snapshot.Stateful
}

// Details is the daemon's record of the snapshot, for showing whole.
func (s *Snapshot) Details() any {
	if s.Volume != nil {
		return s.VolumeSnapshot
	}

	return s.Snapshot
}

func (s *Snapshot) Delete() error {
	var op incus.Operation

	var err error

	if s.Volume != nil {
		op, err = s.Client.DeleteStoragePoolVolumeSnapshot(s.Volume.Pool, s.Volume.Volume.Type, s.Owner, s.Name)
	} else {
		op, err = s.Client.DeleteInstanceSnapshot(s.Owner, s.Name)
	}

	if err != nil {
		return err
	}

	return op.Wait()
}

// Restore rolls the instance or volume back to this snapshot. Incus takes a
// restore as an update to the thing itself, so this reads it first to keep
// the rest of its config intact.
func (s *Snapshot) Restore() error {
	if s.Volume != nil {
		volume, etag, err := s.Client.GetStoragePoolVolume(s.Volume.Pool, s.Volume.Volume.Type, s.Owner)
		if err != nil {
			return err
		}

		put := volume.Writable()
		put.Restore = s.Name

		return s.Client.UpdateStoragePoolVolume(s.Volume.Pool, s.Volume.Volume.Type, s.Owner, put, etag)
	}

	instance, etag, err := s.Client.GetInstance(s.Owner)
	if err != nil {
		return err
	}

	instance.Restore = s.Name

	op, err := s.Client.UpdateInstance(s.Owner, instance.Writable(), etag)
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

// Snapshots are the instance's snapshots as its refresh listed them.
func (i *Instance) Snapshots() []*Snapshot {
	apiSnapshots := i.Instance.Snapshots
	snapshots := make([]*Snapshot, len(apiSnapshots))

	for index := range apiSnapshots {
		apiSnapshot := apiSnapshots[index]

		snapshots[index] = &Snapshot{
			Project:   i.Project,
			Owner:     i.Name,
			Name:      snapshotName(apiSnapshot.Name),
			Snapshot:  apiSnapshot,
			Client:    i.Client,
			OSCommand: i.OSCommand,
			Log:       i.Log,
			Tr:        i.Tr,
		}
	}

	return snapshots
}

// Snapshots are the volume's snapshots as its refresh listed them.
func (v *Volume) Snapshots() []*Snapshot {
	snapshots := make([]*Snapshot, len(v.SnapshotList))

	for index := range v.SnapshotList {
		apiSnapshot := v.SnapshotList[index]

		snapshots[index] = &Snapshot{
			Project:        v.Volume.Project,
			Owner:          v.Name,
			Name:           snapshotName(apiSnapshot.Name),
			Volume:         v,
			VolumeSnapshot: apiSnapshot,
			Client:         v.Client,
			OSCommand:      v.OSCommand,
			Log:            v.Log,
			Tr:             v.Tr,
		}
	}

	return snapshots
}

// CreateSnapshot takes a snapshot of the volume. There's no stateful kind:
// a volume has no runtime state to keep.
func (v *Volume) CreateSnapshot(name string, opts SnapshotOptions) error {
	post := api.StorageVolumeSnapshotsPost{Name: name}

	if opts.ExpiresIn > 0 {
		expiresAt := time.Now().Add(opts.ExpiresIn)
		post.ExpiresAt = &expiresAt
	}

	op, err := v.Client.CreateStoragePoolVolumeSnapshot(v.Pool, v.Volume.Type, v.Name, post)
	if err != nil {
		return err
	}

	return op.Wait()
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
