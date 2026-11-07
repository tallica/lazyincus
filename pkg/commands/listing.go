package commands

import (
	"sync"

	"github.com/lxc/incus/v7/shared/api"
)

// A Listing is one refresh's instance listings, each asked for once: docs/Incus.md, "List instances".
type Listing struct {
	c *IncusCommand
	// withScoped: the panel's listing will be read, so the others can come from it.
	withScoped bool

	scopeOnce sync.Once
	scope     clientScope

	scoped   lazy[[]api.InstanceFull]
	everyone lazy[[]api.Instance]

	mutex    sync.Mutex
	projects map[string]*lazy[[]api.InstanceFull]
}

// NewListing starts a listing, taking the session's scope when first read.
func (c *IncusCommand) NewListing() *Listing {
	return &Listing{c: c, projects: map[string]*lazy[[]api.InstanceFull]{}}
}

// WithScoped says the panel's listing will be read, before any read starts.
func (l *Listing) WithScoped() {
	if l != nil {
		l.withScoped = true
	}
}

// listingOr: a nil Listing, or another remote's, reads afresh.
func (c *IncusCommand) listingOr(l *Listing) *Listing {
	if l == nil || l.c != c {
		return c.NewListing()
	}

	return l
}

func (l *Listing) clientScope() clientScope {
	l.scopeOnce.Do(func() { l.scope = l.c.scope() })

	return l.scope
}

// Scoped is the panels' listing, with each instance's state and snapshots.
func (l *Listing) Scoped() ([]api.InstanceFull, error) {
	return l.scoped.get(func() ([]api.InstanceFull, error) {
		scope := l.clientScope()

		return listInstances(scope.client, scope.allProjects)
	})
}

// AllProjects is every project's instances, without state.
func (l *Listing) AllProjects() ([]api.Instance, error) {
	return l.everyone.get(func() ([]api.Instance, error) {
		if !l.withScoped || !l.clientScope().allProjects {
			return l.clientScope().client.GetInstancesAllProjects(api.InstanceTypeAny)
		}

		fulls, err := l.Scoped()
		if err != nil {
			return nil, err
		}

		instances := make([]api.Instance, len(fulls))
		for i := range fulls {
			instances[i] = fulls[i].Instance
		}

		return instances, nil
	})
}

// Project is one project's instances, with state and snapshots.
func (l *Listing) Project(project string) ([]api.InstanceFull, error) {
	scope := l.clientScope()
	if !scope.allProjects && scope.project == project {
		return l.Scoped()
	}

	l.mutex.Lock()
	listed, ok := l.projects[project]
	if !ok {
		listed = &lazy[[]api.InstanceFull]{}
		l.projects[project] = listed
	}
	l.mutex.Unlock()

	return listed.get(func() ([]api.InstanceFull, error) {
		if !l.withScoped || !scope.allProjects {
			return scope.base.UseProject(project).GetInstancesFull(api.InstanceTypeAny)
		}

		fulls, err := l.Scoped()
		if err != nil {
			return nil, err
		}

		var inProject []api.InstanceFull

		for _, full := range fulls {
			if full.Project == project {
				inProject = append(inProject, full)
			}
		}

		return inProject, nil
	})
}

// lazy is a value read once, by whichever caller asks first.
type lazy[T any] struct {
	once  sync.Once
	value T
	err   error
}

func (v *lazy[T]) get(read func() (T, error)) (T, error) {
	v.once.Do(func() { v.value, v.err = read() })

	return v.value, v.err
}
