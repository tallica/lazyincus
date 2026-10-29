package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// A fetch runs off the loop; Update makes it, issuing its ticket, and runs apply.
type fetch func() (apply func(), err error)

type refreshMsg struct {
	applies []func()
	err     error
	// quiet logs err instead of reporting it: a poll's.
	quiet bool
	// round counts refreshAll's groups yet to land.
	round *int
	// next runs whatever the fetches found: a poll re-arming.
	next tea.Cmd
}

// refresh is pkg/gui's refresh as a Cmd.
func refresh(fetches ...fetch) tea.Cmd {
	return func() tea.Msg {
		applies, err := gather(fetches)

		return refreshMsg{applies: applies, err: err}
	}
}

// refreshQuiet is refresh for a poll, which then runs next.
func refreshQuiet(next tea.Cmd, fetches ...fetch) tea.Cmd {
	return func() tea.Msg {
		applies, err := gather(fetches)

		return refreshMsg{applies: applies, err: err, quiet: true, next: next}
	}
}

func gather(fetches []fetch) ([]func(), error) {
	applies := make([]func(), 0, len(fetches))

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

// refreshAll reads every panel, each group shown as soon as it's read.
func (m *Model) refreshAll() tea.Cmd {
	groups := m.fetchGroups()
	round := len(groups)

	cmds := make([]tea.Cmd, len(groups))
	for i, group := range groups {
		cmds[i] = func() tea.Msg {
			applies, err := gather(group)

			return refreshMsg{applies: applies, err: err, quiet: true, round: &round}
		}
	}

	return tea.Batch(cmds...)
}

// fetchGroups is pkg/gui's, for the panels ported so far.
func (m *Model) fetchGroups() [][]fetch {
	return [][]fetch{
		{m.fetchInstances()},
	}
}

func (m *Model) applyRefresh(msg refreshMsg) tea.Cmd {
	for _, apply := range msg.applies {
		apply()
	}

	if msg.round != nil {
		*msg.round--
		if *msg.round == 0 {
			m.stopAwaiting()
		}
	}

	if msg.err != nil {
		if msg.quiet {
			m.incus.Log.Warn().Err(msg.err).Send()
		} else {
			m.handleError(msg.err)
		}
	}

	return msg.next
}

// stopAwaiting has a panel no read answered say it's empty, not loading.
func (m *Model) stopAwaiting() {
	for _, p := range m.panels {
		if p.awaiting != "" {
			p.awaiting = ""
			p.rowsWidth = -1
		}
	}
}

// handleError is pkg/gui's: an unreachable daemon is the footer's to report.
func (m *Model) handleError(err error) {
	m.incus.Log.Error().Err(err).Send()

	if commands.IsConnectionError(err) {
		m.incus.NoteError(err)
		return
	}

	m.openError(err.Error())
}

// tick is tea.Tick unless a test has replaced it to fire ticks itself.
// nolint:unparam
func (m *Model) tick(d time.Duration, msg tea.Msg) tea.Cmd {
	if m.after != nil {
		return m.after(d, msg)
	}

	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// refreshSeq keeps a slow fetch from undoing a later one of the same kind.
type refreshSeq struct {
	issued, applied uint64
}

func (s *refreshSeq) issue() uint64 {
	s.issued++

	return s.issued
}

func (s *refreshSeq) admit(ticket uint64) bool {
	if ticket < s.applied {
		return false
	}

	s.applied = ticket

	return true
}

type refreshSeqs struct {
	instances refreshSeq
}
