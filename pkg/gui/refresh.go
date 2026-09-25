package gui

import (
	"sync/atomic"

	"github.com/jesseduffield/gocui"
)

// A fetch asks the daemon for one panel's contents, off the main loop, and
// returns what shows them - which only the main loop may run: panels,
// views and gui.State are its alone.
type fetch func() (apply func() error, err error)

// refresh runs the fetches here and applies what they found on the main
// loop, in order, then runs then there too. The caller is never the main
// loop: a keypress that wants a refresh starts it on a goroutine, or from
// inside WithWaitingStatus, which is one.
//
// A failed fetch doesn't hold back the others; its error is returned, and
// then is skipped - it could move focus out from under the error popup.
func (gui *Gui) refresh(then func() error, fetches ...fetch) error {
	applies := make([]func() error, 0, len(fetches))

	var firstErr error

	for _, fetch := range fetches {
		apply, err := fetch()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}

			continue
		}

		applies = append(applies, apply)
	}

	if firstErr != nil {
		then = nil
	}

	gui.g.Update(func(*gocui.Gui) error {
		for _, apply := range applies {
			if err := apply(); err != nil {
				return err
			}
		}

		if then == nil {
			return nil
		}

		return then()
	})

	return firstErr
}

// refreshInstancesAndServices re-lists both panels as soon as something has
// changed what they hold, rather than waiting on the next background poll
// to notice. Both, because a compose instance has a row in each.
func (gui *Gui) refreshInstancesAndServices() error {
	return gui.refresh(nil, gui.fetchInstances, gui.fetchServices)
}

// refreshInBackground is refresh for a caller on the main loop. A failure
// goes through the same handler as a keypress's.
func (gui *Gui) refreshInBackground(fetches ...fetch) {
	go func() {
		if err := gui.refresh(nil, fetches...); err != nil {
			gui.g.Update(func(*gocui.Gui) error { return err })
		}
	}()
}

// refreshSeq keeps a slow fetch from undoing a later one of the same kind:
// the poll and an action's own refresh overlap, and gocui runs updates in
// no particular order.
type refreshSeq struct {
	issued atomic.Uint64
	// applied is the main loop's.
	applied uint64
}

func (s *refreshSeq) issue() uint64 {
	return s.issued.Add(1)
}

// admit reports whether a fetch's result is still the newest. Main loop only.
func (s *refreshSeq) admit(ticket uint64) bool {
	if ticket < s.applied {
		return false
	}

	s.applied = ticket

	return true
}

// invalidate turns away every fetch already in flight, for a change of scope
// that makes their results wrong rather than old. Main loop only.
func (s *refreshSeq) invalidate() {
	s.applied = s.issued.Add(1)
}

// refreshSeqs is one refreshSeq per kind of fetch.
type refreshSeqs struct {
	instances refreshSeq
	images    refreshSeq
	volumes   refreshSeq
	networks  refreshSeq
	services  refreshSeq
}

func (s *refreshSeqs) invalidateAll() {
	for _, seq := range []*refreshSeq{&s.instances, &s.images, &s.volumes, &s.networks, &s.services} {
		seq.invalidate()
	}
}
