package tui

import (
	"maps"
	"slices"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
)

const operationsKept = 100

// operationLog is pkg/gui's: the daemon forgets an operation 5s after it ends, so its events are the history.
type operationLog struct {
	entries map[string]*loggedOperation
}

type loggedOperation struct {
	operation *commands.Operation
	// seen is by lazyincus's clock: the daemon's can be off.
	seen  time.Time
	first time.Time
}

// operationsShownAfter keeps the footer's count from blinking: most operations end in less.
const operationsShownAfter = time.Second

// operationRank keeps a late event from taking a status back (docs/Incus.md, "Events").
func operationRank(operation *commands.Operation) int {
	switch {
	case operation.IsFinal():
		return 2
	case operation.Operation.StatusCode == api.Running:
		return 1
	default:
		return 0
	}
}

func (l *operationLog) record(operation *commands.Operation, at time.Time) {
	if operation.IsToken() {
		return
	}

	if l.entries == nil {
		l.entries = map[string]*loggedOperation{}
	}

	if logged, ok := l.entries[operation.Key()]; ok {
		have, got := operationRank(logged.operation), operationRank(operation)
		if have > got || (have == got && logged.operation.Operation.UpdatedAt.After(operation.Operation.UpdatedAt)) {
			return
		}

		// A listing doesn't say when it ended; the event did.
		if operation.Ended.IsZero() && !logged.operation.Ended.IsZero() {
			kept := *operation
			kept.Ended = logged.operation.Ended
			operation = &kept
		}
	}

	first := at
	if logged, ok := l.entries[operation.Key()]; ok {
		first = logged.first
	}

	l.entries[operation.Key()] = &loggedOperation{operation: operation, seen: at, first: first}
	l.prune()
}

// reconcile drops what's under way in the log but not listed, unless heard of since asked: it ended while the stream was down.
func (l *operationLog) reconcile(listed []*commands.Operation, asked, at time.Time) {
	ids := map[string]bool{}

	for _, operation := range listed {
		ids[operation.Key()] = true
		l.record(operation, at)
	}

	for id, logged := range l.entries {
		if !ids[id] && !logged.operation.IsFinal() && logged.seen.Before(asked) {
			delete(l.entries, id)
		}
	}
}

func (l *operationLog) prune() {
	ended := lo.Filter(lo.Values(l.entries), func(logged *loggedOperation, _ int) bool {
		return logged.operation.IsFinal()
	})

	if len(ended) <= operationsKept {
		return
	}

	slices.SortFunc(ended, func(a, b *loggedOperation) int {
		return b.operation.Operation.UpdatedAt.Compare(a.operation.Operation.UpdatedAt)
	})

	for _, logged := range ended[operationsKept:] {
		delete(l.entries, logged.operation.Key())
	}
}

func (l *operationLog) list() []*commands.Operation {
	return lo.Map(lo.Values(l.entries), func(logged *loggedOperation, _ int) *commands.Operation { return logged.operation })
}

// counts' due is when the next running one is old enough to count, zero if none.
func (l *operationLog) counts(since, now time.Time) (running, failed int, due time.Time) {
	for _, logged := range l.entries {
		switch {
		case !logged.operation.IsFinal() && now.Sub(logged.first) < operationsShownAfter:
			if at := logged.first.Add(operationsShownAfter); due.IsZero() || at.Before(due) {
				due = at
			}
		case !logged.operation.IsFinal():
			running++
		case logged.operation.Operation.StatusCode == api.Failure && logged.seen.After(since) &&
			!l.retried(logged.operation):
			failed++
		}
	}

	return running, failed, due
}

const retriedWithin = time.Minute

// retried: incus-compose retries a delete the daemon refused while ic-healthd wrote.
func (l *operationLog) retried(failure *commands.Operation) bool {
	if failure.Operation.StatusCode != api.Failure {
		return false
	}

	for _, logged := range l.entries {
		other := logged.operation
		if other.Operation.StatusCode == api.Success && other.Project == failure.Project &&
			other.Operation.Description == failure.Operation.Description &&
			other.Operation.CreatedAt.After(failure.Operation.CreatedAt) &&
			other.Operation.CreatedAt.Sub(failure.Operation.CreatedAt) <= retriedWithin &&
			maps.EqualFunc(other.Operation.Resources, failure.Operation.Resources, slices.Equal) {
			return true
		}
	}

	return false
}

func (l *operationLog) clear() {
	l.entries = nil
}
