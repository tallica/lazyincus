// Package incustest is an Incus daemon for tests: a client that answers the
// listing calls lazyincus makes from fixed data.
package incustest

import (
	"errors"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

// Server answers the listing calls lazyincus makes from its fields. It
// embeds the interface it stands in for, left nil, so a call it doesn't
// implement panics rather than quietly returning nothing. Make one with New.
type Server struct {
	incus.InstanceServer

	Instances []api.InstanceFull
	Images    []api.Image
	Networks  []api.Network
	Profiles  []api.Profile
	// Volumes by storage pool.
	Volumes map[string][]api.StorageVolume
	// PoolSpace by pool; VolumeUsage in bytes and VolumeSnapshots by volume
	// name.
	PoolSpace       map[string]api.ResourcesStoragePoolSpace
	VolumeUsage     map[string]uint64
	VolumeSnapshots map[string][]api.StorageVolumeSnapshot
	// NetworkLeases by network, then by the project that sees them - the
	// daemon lists only the asking project's. NetworkStates by network. A
	// network missing from either is not found, as leases on an unmanaged
	// one are.
	NetworkLeases map[string]map[string][]api.NetworkLease
	NetworkStates map[string]api.NetworkState
	// ConsoleLogs by instance name: what each read of its console log
	// returns.
	ConsoleLogs map[string]string
	// NetworkACLs by name, NetworkForwards by network.
	NetworkACLs     map[string]api.NetworkACL
	NetworkForwards map[string][]api.NetworkForward
	// InstancesError fails the instance listings alone, as for a client
	// allowed the images but not the instances.
	InstancesError error

	// project is what UseProject scoped this copy to.
	project string

	// state is shared by every copy UseProject makes: what a test changes
	// while the app is running.
	state *state
}

type state struct {
	mutex     sync.Mutex
	down      bool
	instances []api.InstanceFull
	changed   bool
	images    []api.Image
	// imagesSet is changed's counterpart for images.
	imagesSet bool
	listeners []*listener
}

// New is a Server answering from fixture.
func New(fixture Server) *Server {
	fixture.state = &state{}

	return &fixture
}

func (s *Server) shared() *state {
	return s.state
}

// SetInstances replaces the instances while the app is running.
func (s *Server) SetInstances(instances []api.InstanceFull) {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	shared.instances = instances
	shared.changed = true
}

// SetImages replaces the images while the app is running.
func (s *Server) SetImages(images []api.Image) {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	shared.images = images
	shared.imagesSet = true
}

func (s *Server) images() []api.Image {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	if shared.imagesSet {
		return slices.Clone(shared.images)
	}

	return slices.Clone(s.Images)
}

// SetDown makes every listing fail the way an unreachable daemon's does,
// and drops the open event streams.
func (s *Server) SetDown(down bool) {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	shared.down = down

	if down {
		shared.dropListeners()
	}
}

// errUnreachable is what the client returns for a daemon it never reached.
var errUnreachable = &url.Error{Op: "Get", URL: "https://incustest/1.0", Err: errors.New("connection refused")}

// reachable is errUnreachable while the daemon is down.
func (s *Server) reachable() error {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	if shared.down {
		return errUnreachable
	}

	return nil
}

func (s *Server) instances() ([]api.InstanceFull, error) {
	shared := s.shared()
	shared.mutex.Lock()
	defer shared.mutex.Unlock()

	if shared.down {
		return nil, errUnreachable
	}

	if s.InstancesError != nil {
		return nil, s.InstancesError
	}

	if shared.changed {
		return slices.Clone(shared.instances), nil
	}

	return slices.Clone(s.Instances), nil
}

var _ incus.InstanceServer = &Server{}

func (s *Server) scope() string {
	if s.project == "" {
		return api.ProjectDefaultName
	}

	return s.project
}

func inProject[T any](items []T, project string, projectOf func(T) string) []T {
	return slices.DeleteFunc(slices.Clone(items), func(item T) bool { return projectOf(item) != project })
}

func (s *Server) UseProject(name string) incus.InstanceServer {
	scoped := *s
	scoped.project = name

	return &scoped
}

func (s *Server) GetConnectionInfo() (*incus.ConnectionInfo, error) {
	return &incus.ConnectionInfo{Project: s.scope()}, nil
}

func (s *Server) GetProjectNames() ([]string, error) {
	instances, err := s.instances()
	if err != nil {
		return nil, err
	}

	names := []string{api.ProjectDefaultName}

	for _, instance := range instances {
		if !slices.Contains(names, instance.Project) {
			names = append(names, instance.Project)
		}
	}

	return names, nil
}

func (s *Server) GetInstancesFull(api.InstanceType) ([]api.InstanceFull, error) {
	instances, err := s.instances()
	if err != nil {
		return nil, err
	}

	return inProject(instances, s.scope(), func(i api.InstanceFull) string { return i.Project }), nil
}

func (s *Server) GetInstancesFullAllProjects(api.InstanceType) ([]api.InstanceFull, error) {
	return s.instances()
}

func (s *Server) GetInstances(instanceType api.InstanceType) ([]api.Instance, error) {
	instances, err := s.GetInstancesFull(instanceType)

	return plain(instances), err
}

func (s *Server) GetInstancesAllProjects(instanceType api.InstanceType) ([]api.Instance, error) {
	instances, err := s.GetInstancesFullAllProjects(instanceType)

	return plain(instances), err
}

func plain(instances []api.InstanceFull) []api.Instance {
	out := make([]api.Instance, len(instances))
	for i, instance := range instances {
		out[i] = instance.Instance
	}

	return out
}

func (s *Server) GetImages() ([]api.Image, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return inProject(s.images(), s.scope(), func(i api.Image) string { return i.Project }), nil
}

func (s *Server) GetImagesAllProjects() ([]api.Image, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return s.images(), nil
}

func (s *Server) GetProfiles() ([]api.Profile, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return inProject(s.Profiles, s.scope(), func(p api.Profile) string { return p.Project }), nil
}

func (s *Server) GetProfilesAllProjects() ([]api.Profile, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return slices.Clone(s.Profiles), nil
}

func (s *Server) GetNetworks() ([]api.Network, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return inProject(s.Networks, s.scope(), func(n api.Network) string { return n.Project }), nil
}

func (s *Server) GetNetworksAllProjects() ([]api.Network, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	return slices.Clone(s.Networks), nil
}

// errNetworkNotFound is the daemon's answer for leases on a network it
// doesn't manage.
var errNetworkNotFound = api.StatusErrorf(404, "Network not found")

func (s *Server) GetNetworkLeases(name string) ([]api.NetworkLease, error) {
	byProject, ok := s.NetworkLeases[name]
	if !ok {
		return nil, errNetworkNotFound
	}

	return slices.Clone(byProject[s.scope()]), nil
}

func (s *Server) GetNetworkState(name string) (*api.NetworkState, error) {
	state, ok := s.NetworkStates[name]
	if !ok {
		return nil, errNetworkNotFound
	}

	return &state, nil
}

func (s *Server) GetNetworkACL(name string) (*api.NetworkACL, string, error) {
	acl, ok := s.NetworkACLs[name]
	if !ok {
		return nil, "", api.StatusErrorf(404, "Network ACL not found")
	}

	return &acl, "", nil
}

func (s *Server) GetNetworkForwards(network string) ([]api.NetworkForward, error) {
	return slices.Clone(s.NetworkForwards[network]), nil
}

func (s *Server) GetStoragePools() ([]api.StoragePool, error) {
	if err := s.reachable(); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(s.Volumes))
	for pool := range s.Volumes {
		names = append(names, pool)
	}

	slices.Sort(names)

	pools := make([]api.StoragePool, len(names))
	for i, name := range names {
		pools[i] = api.StoragePool{Name: name, Driver: "dir"}
	}

	return pools, nil
}

func (s *Server) GetStoragePoolVolumeSnapshots(_, _, name string) ([]api.StorageVolumeSnapshot, error) {
	return slices.Clone(s.VolumeSnapshots[name]), nil
}

func (s *Server) GetStoragePoolResources(pool string) (*api.ResourcesStoragePool, error) {
	space, ok := s.PoolSpace[pool]
	if !ok {
		return nil, api.StatusErrorf(404, "Storage pool not found")
	}

	return &api.ResourcesStoragePool{Space: space}, nil
}

// GetStoragePoolVolumeState answers from VolumeUsage, by volume name; a
// volume missing from it has no usage, as on a dir pool without quotas.
func (s *Server) GetStoragePoolVolumeState(_, _, name string) (*api.StorageVolumeState, error) {
	usage := &api.StorageVolumeStateUsage{}
	if used, ok := s.VolumeUsage[name]; ok {
		usage.Used = used
	}

	return &api.StorageVolumeState{Usage: usage}, nil
}

func (s *Server) GetStoragePoolVolumes(pool string) ([]api.StorageVolume, error) {
	return inProject(s.Volumes[pool], s.scope(), func(v api.StorageVolume) string { return v.Project }), nil
}

func (s *Server) GetStoragePoolVolumesAllProjects(pool string) ([]api.StorageVolume, error) {
	return slices.Clone(s.Volumes[pool]), nil
}

func (s *Server) GetInstanceConsoleLog(name string, _ *incus.InstanceConsoleLogArgs) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.ConsoleLogs[name])), nil
}
