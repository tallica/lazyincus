package gui

import (
	"strings"
	"sync"

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
}

func (f *composeFailures) watch(project string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.watching, f.project, f.errs = true, project, nil
}

// stop ends the watch and returns what failed during it.
func (f *composeFailures) stop() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	errs := f.errs
	f.watching, f.project, f.errs = false, "", nil

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
}
