package gui

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
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
	applies, err := gather(fetches)
	if err != nil {
		then = nil
	}

	gui.g.Update(func(*gocui.Gui) error {
		if err := applyAll(applies); err != nil {
			return err
		}

		if then == nil {
			return nil
		}

		return then()
	})

	return err
}

// gather runs the fetches, returning what shows the ones that succeeded and
// the first error.
func gather(fetches []fetch) ([]func() error, error) {
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

	return applies, firstErr
}

func applyAll(applies []func() error) error {
	for _, apply := range applies {
		if err := apply(); err != nil {
			return err
		}
	}

	return nil
}

// refreshAll reads every panel, at startup or on a change of scope: each
// group shown as soon as it's read, so a slow remote's images don't hold
// back its instances. Every group's error is returned.
func (gui *Gui) refreshAll() []error {
	groups := gui.fetchGroups()
	errs := make([]error, len(groups))

	// Main loop only. Update doesn't keep order, so the last group shown,
	// not one more Update, is what clears what the reads left unanswered.
	unshown := len(groups)

	var wg sync.WaitGroup
	for i, group := range groups {
		wg.Go(func() {
			applies, err := gather(group)
			errs[i] = err

			gui.g.Update(func(*gocui.Gui) error {
				unshown--

				err := applyAll(applies)
				if unshown > 0 {
					return err
				}

				return errors.Join(err, gui.stopAwaiting())
			})
		})
	}

	wg.Wait()

	return lo.Compact(errs)
}

// stopAwaiting has a panel no read answered say it's empty, not loading.
func (gui *Gui) stopAwaiting() error {
	for _, panel := range gui.allSidePanels() {
		if !panel.StopAwaiting() {
			continue
		}

		if err := panel.RerenderList(); err != nil {
			return err
		}
	}

	return nil
}

// refreshInstancesAndServices re-lists both panels as soon as something has
// changed what they hold, rather than waiting on the next background poll
// to notice. Both, because a compose instance has a row in each.
func (gui *Gui) refreshInstancesAndServices() error {
	return gui.refresh(nil, gui.fetchInstances, gui.fetchServices)
}

// refreshEnding refreshes, then calls every end and redraws the rows they
// marked: once the listing has landed, or without it when a fetch fails or
// applying one does. refresh's then runs in neither case, and a mark that
// nothing ends stays on its row.
func (gui *Gui) refreshEnding(ends []func(), fetches ...fetch) error {
	ended := func() error {
		for _, end := range ends {
			end()
		}

		return gui.rerenderInstanceLists()
	}

	guarded := make([]fetch, len(fetches))
	for i, fetch := range fetches {
		guarded[i] = func() (func() error, error) {
			apply, err := fetch()
			if err != nil {
				return nil, err
			}

			return func() error {
				if err := apply(); err != nil {
					_ = ended()
					return err
				}

				return nil
			}, nil
		}
	}

	err := gui.refresh(ended, guarded...)
	if err != nil {
		gui.g.Update(func(*gocui.Gui) error { return ended() })
	}

	return err
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
	profiles  refreshSeq
	services  refreshSeq
	stacks    refreshSeq
}

func (s *refreshSeqs) invalidateAll() {
	for _, seq := range []*refreshSeq{&s.instances, &s.images, &s.volumes, &s.networks, &s.profiles, &s.services, &s.stacks} {
		seq.invalidate()
	}
}
