// Package incustest is an Incus daemon for tests: a client that answers the
// listing calls lazyincus makes from fixed data.
package incustest

import (
	"slices"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

// Server answers the listing calls lazyincus makes from its fields. It
// embeds the interface it stands in for, left nil, so a call it doesn't
// implement panics rather than quietly returning nothing.
type Server struct {
	incus.InstanceServer

	Instances []api.InstanceFull
	Images    []api.Image
	Networks  []api.Network
	// Volumes by storage pool.
	Volumes map[string][]api.StorageVolume

	// project is what UseProject scoped this copy to.
	project string
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
	names := []string{api.ProjectDefaultName}

	for _, instance := range s.Instances {
		if !slices.Contains(names, instance.Project) {
			names = append(names, instance.Project)
		}
	}

	return names, nil
}

func (s *Server) GetInstancesFull(api.InstanceType) ([]api.InstanceFull, error) {
	return inProject(s.Instances, s.scope(), func(i api.InstanceFull) string { return i.Project }), nil
}

func (s *Server) GetInstancesFullAllProjects(api.InstanceType) ([]api.InstanceFull, error) {
	return slices.Clone(s.Instances), nil
}

func (s *Server) GetImages() ([]api.Image, error) {
	return inProject(s.Images, s.scope(), func(i api.Image) string { return i.Project }), nil
}

func (s *Server) GetImagesAllProjects() ([]api.Image, error) {
	return slices.Clone(s.Images), nil
}

func (s *Server) GetNetworks() ([]api.Network, error) {
	return inProject(s.Networks, s.scope(), func(n api.Network) string { return n.Project }), nil
}

func (s *Server) GetNetworksAllProjects() ([]api.Network, error) {
	return slices.Clone(s.Networks), nil
}

func (s *Server) GetStoragePoolNames() ([]string, error) {
	names := make([]string, 0, len(s.Volumes))
	for pool := range s.Volumes {
		names = append(names, pool)
	}

	slices.Sort(names)

	return names, nil
}

func (s *Server) GetStoragePoolVolumes(pool string) ([]api.StorageVolume, error) {
	return inProject(s.Volumes[pool], s.scope(), func(v api.StorageVolume) string { return v.Project }), nil
}

func (s *Server) GetStoragePoolVolumesAllProjects(pool string) ([]api.StorageVolume, error) {
	return slices.Clone(s.Volumes[pool]), nil
}
