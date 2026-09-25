package commands

import (
	"strings"

	"github.com/sasha-s/go-deadlock"
)

// instanceRuntime is what outlives the Instance each refresh replaces: the
// newest of those, the console log drained from it so far - the endpoint
// hands each byte out once - and which `ps` worked in it.
type instanceRuntime struct {
	mutex             deadlock.Mutex
	latest            *Instance
	logBuffer         strings.Builder
	stoppedLogFetched bool
	topCommand        []string
	// listing is the newest listing that has seen the instance.
	listing uint64
}

// instanceRuntimes holds one instanceRuntime per instance, by Instance.Key.
type instanceRuntimes struct {
	mutex    deadlock.Mutex
	byKey    map[string]*instanceRuntime
	listings uint64
}

// beginListing numbers a listing before it asks the daemon, so that one
// answering late can't pass off what it saw as newer than a later one's.
func (r *instanceRuntimes) beginListing() uint64 {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.listings++

	return r.listings
}

// attach gives the instance its runtime, and makes it the newest the
// runtime knows of - unless a later listing already has.
func (r *instanceRuntimes) attach(instance *Instance, listing uint64) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.byKey == nil {
		r.byKey = map[string]*instanceRuntime{}
	}

	runtime, ok := r.byKey[instance.Key()]
	if !ok {
		runtime = &instanceRuntime{}
		r.byKey[instance.Key()] = runtime
	}

	runtime.mutex.Lock()
	if runtime.latest == nil || listing >= runtime.listing {
		runtime.latest = instance
		runtime.listing = listing
	}
	runtime.mutex.Unlock()

	instance.runtime = runtime
}

// prune drops the runtimes of instances a listing no longer has. A listing
// only speaks for the projects it covered, and not for an instance a later
// listing has seen, so the rest are left alone.
func (r *instanceRuntimes) prune(covers func(project string) bool, listed []*Instance, listing uint64) {
	keep := make(map[string]bool, len(listed))
	for _, instance := range listed {
		keep[instance.Key()] = true
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	for key, runtime := range r.byKey {
		runtime.mutex.Lock()
		project := runtime.latest.Project
		newer := runtime.listing > listing
		runtime.mutex.Unlock()

		if covers(project) && !keep[key] && !newer {
			delete(r.byKey, key)
		}
	}
}
