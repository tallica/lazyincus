package gui

import (
	"strings"
	"sync"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/tallica/lazyincus/pkg/commands"
)

// composeFailures collects the operations that fail in a stack's project
// while a compose verb has the terminal (docs/Panels.md, "Services").
type composeFailures struct {
	mutex    sync.Mutex
	watching bool
	project  string
	errs     []string
	// failed closes with the first failure the watch keeps.
	failed chan struct{}
}

// composeFailureGrace is how long a failed verb's failure may trail its
// exit: the daemon's event and its answer to incus-compose come down
// different connections.
const composeFailureGrace = 200 * time.Millisecond

func (f *composeFailures) watch(project string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.watching, f.project, f.errs, f.failed = true, project, nil, make(chan struct{})
}

// stop ends the watch and returns what failed during it, waiting up to
// grace for a first failure if none has arrived.
func (f *composeFailures) stop(grace time.Duration) []string {
	f.mutex.Lock()
	watching, failed := f.watching, f.failed
	f.mutex.Unlock()

	if watching && grace > 0 {
		select {
		case <-failed:
		case <-time.After(grace):
		}
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	errs := f.errs
	f.watching, f.project, f.errs, f.failed = false, "", nil, nil

	return errs
}

// note takes in an operation event, keeping it if it's a failure in the
// project being watched.
func (f *composeFailures) note(event commands.Event) {
	if event.Status != api.Failure || event.Details == nil || event.Details.Operation.Err == "" {
		return
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.watching || event.Project != f.project {
		return
	}

	what := strings.TrimSpace(event.Action + " " + strings.Join(event.Instances, ", "))
	f.errs = append(f.errs, what+": "+event.Details.Operation.Err)

	if len(f.errs) == 1 {
		close(f.failed)
	}
}
