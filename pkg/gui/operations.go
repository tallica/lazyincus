package gui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

// operationsKept is how many ended operations the log keeps.
const operationsKept = 100

// operationLog is every operation lazyincus has seen in the session's scope:
// the daemon forgets one 5s after it ends, so its events are the history.
// Main loop only.
type operationLog struct {
	entries map[string]*loggedOperation
}

type loggedOperation struct {
	operation *commands.Operation
	// seen is when lazyincus last heard of it, by its own clock: the
	// daemon's can be off.
	seen time.Time
	// first is when it was first heard of, which the footer waits on.
	first time.Time
}

// operationsShownAfter is how long an operation runs before the footer
// counts it: most end in less, and a count that blinks says nothing.
const operationsShownAfter = time.Second

// operationRank orders the statuses an operation goes through, so an event
// that arrives late can't take one back: the daemon can report Running
// after Success (docs/Incus.md, "Events"), and gocui runs updates in no
// particular order.
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

// record takes in what an event or a listing says of an operation, unless
// what the log has is further along, or as far along and newer.
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

// reconcile takes in a listing asked for at asked. An operation the log has
// as under way that the listing doesn't have ended while no event said so -
// the stream was down - and how is unknown, so it goes; one heard of since
// the listing was asked for stays.
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

// prune drops the oldest ended operations past operationsKept.
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

// counts is how many operations have been under way for operationsShownAfter,
// and how many failed after since; due is when the next younger one is old
// enough to count, zero if none is waiting.
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

// retriedWithin is how soon after a failure a success counts as its retry.
const retriedWithin = time.Minute

// retried is whether the same action on the same resources went through
// soon after: incus-compose retries a delete refused while ic-healthd writes.
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

// operationsShown is which operations the list is cut down to.
type operationsShown int

const (
	operationsAll operationsShown = iota
	operationsFailed
	operationsSucceeded
)

func (shown operationsShown) has(operation *commands.Operation) bool {
	switch shown {
	case operationsFailed:
		return operation.Operation.StatusCode == api.Failure
	case operationsSucceeded:
		return operation.Operation.StatusCode == api.Success
	default:
		return true
	}
}

// operationsShownLabel is what the list is cut down to, "" for nothing.
func (gui *Gui) operationsShownLabel() string {
	switch gui.State.OperationsShown {
	case operationsFailed:
		return gui.Tr.OperationsFailedOnly
	case operationsSucceeded:
		return gui.Tr.OperationsSucceededOnly
	default:
		return ""
	}
}

// handleOperationsShown is `f` on the operations: all, failed, succeeded.
func (gui *Gui) handleOperationsShown(*gocui.Gui, *gocui.View) error {
	if !gui.daemonOpen(daemonOperations) {
		return nil
	}

	selected := gui.daemonSelected(daemonOperations)
	gui.State.OperationsShown = (gui.State.OperationsShown + 1) % (operationsSucceeded + 1)

	return gui.redrawDaemon(daemonOperations, selected)
}

// operationItems are the popup's rows, under way first, being what the list
// is for, then newest first.
func (gui *Gui) operationItems() []*types.MenuItem {
	operations := lo.Filter(gui.operations.list(), func(operation *commands.Operation, _ int) bool {
		return gui.State.OperationsShown.has(operation)
	})
	slices.SortStableFunc(operations, func(a, b *commands.Operation) int {
		if a.IsFinal() != b.IsFinal() {
			if a.IsFinal() {
				return 1
			}

			return -1
		}

		return b.Operation.CreatedAt.Compare(a.Operation.CreatedAt)
	})

	items := make([]*types.MenuItem, 0, len(operations))

	for _, operation := range operations {
		columns := presentation.GetOperationDisplayStrings(
			operation, gui.State.SpansProjects.Operations, gui.operations.retried(operation))
		items = append(items, &types.MenuItem{
			Key:          operation.Key(),
			LabelColumns: columns,
			FilterText:   operationLabel(operation) + " " + operation.Project,
			OnPress:      func() error { return gui.showOperation(operation) },
			Keys: map[rune]types.MenuKey{
				'd': {Handler: func() error { return gui.operationCancel(operation) }, Mutates: true},
			},
		})
	}

	if len(items) == 0 {
		empty := gui.Tr.NoOperations
		if shown := gui.operationsShownLabel(); shown != "" {
			empty = fmt.Sprintf(gui.Tr.NoOperationsShown, shown)
		}

		items = append(items, &types.MenuItem{LabelColumns: []string{empty}})
	}

	return items
}

// showOperation is enter on an operation: all of it, which the row has no
// room for. Closing it goes back to the list.
func (gui *Gui) showOperation(operation *commands.Operation) error {
	back := func(*gocui.Gui, *gocui.View) error { return gui.openDaemon(daemonOperations, operation.Key()) }

	return gui.createMessagePanel(gui.Tr.OperationTitle, strings.TrimRight(gui.operationInfoStr(operation), "\n"), back)
}

func (gui *Gui) operationInfoStr(operation *commands.Operation) string {
	op := operation.Operation
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", identityPadding) + value + "\n"
	}

	output := gui.locationStr(gui.sessionLocation(operation.Project))
	output += line("Description", op.Description)
	output += line("Status", presentation.OperationStatus(operation, gui.operations.retried(operation)))
	if op.Err != "" {
		output += line("Error", utils.ColoredString(op.Err, color.FgRed))
	}

	if !operation.IsFinal() {
		output += line("Progress", operation.Progress())
	}
	output += line("Class", op.Class)
	output += line("Started", op.CreatedAt.Local().Format(presentation.SecondsFormat))

	if operation.IsFinal() {
		output += line("Took", presentation.OperationDuration(operation.Took()))
	} else {
		output += line("Updated", op.UpdatedAt.Local().Format(presentation.SecondsFormat))
	}

	output += line("Cancellable", gui.yesNo(op.MayCancel && !operation.IsFinal()))
	// A daemon that isn't clustered says "none".
	if op.Location != "none" {
		output += line("Member", op.Location)
	}

	// Plain headings: sectionHeading rules to the main panel's width, wider
	// than a popup.
	if resources := operation.Resources(); len(resources) > 0 {
		output += "\n" + gui.Tr.OperationResources + ":\n" + strings.Join(resources, "\n") + "\n"
	}

	// A websocket's metadata is its connection secrets.
	if op.Class == "task" && len(op.Metadata) > 0 {
		data, err := utils.MarshalIntoYaml(op.Metadata)
		if err == nil {
			output += "\n" + gui.Tr.OperationMetadata + ":\n" + utils.ColoredYamlString(string(data))
		}
	}

	return output
}

// fetchOperations reconciles the log with what the daemon holds.
func (gui *Gui) fetchOperations() (func() error, error) {
	ticket := gui.refreshes.operations.issue()
	asked := time.Now()

	operations, err := gui.IncusCommand.GetOperations()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.operations.admit(ticket) {
			return nil
		}

		selected := gui.daemonSelected(daemonOperations)
		gui.operations.reconcile(operations, asked, time.Now())

		return gui.showOperations(selected)
	}, nil
}

func (gui *Gui) refreshOperationsQuiet() error {
	if err := gui.refresh(nil, gui.fetchOperations); err != nil {
		gui.Log.Warn().Err(err).Send()
	}

	return nil
}

// recordOperation takes an operation event into the log, unless the scope
// its stream was opened for has been left since: the old stream closes
// after the log is cleared, and can still deliver. Off the main loop.
func (gui *Gui) recordOperation(scope uint64, operation *commands.Operation) {
	at := time.Now()

	gui.g.Update(func(*gocui.Gui) error {
		if gui.operationsScope.Load() != scope {
			return nil
		}

		selected := gui.daemonSelected(daemonOperations)
		gui.operations.record(operation, at)

		return gui.showOperations(selected)
	})
}

// showOperations redraws the popup if it's open on the log, and says in
// the footer what's running and what has failed unseen. selected is the
// row the popup had selected before the log changed. Main loop only.
func (gui *Gui) showOperations(selected string) error {
	gui.State.SpansProjects.Operations = spansMultipleProjects(
		lo.Map(gui.operations.list(), func(operation *commands.Operation, _ int) string { return operation.Project }))

	if err := gui.redrawDaemon(daemonOperations, selected); err != nil {
		return err
	}

	return gui.countOperations()
}

// countOperations hands the footer its counts, which it reads off the main
// loop, and redraws it.
func (gui *Gui) countOperations() error {
	now := time.Now()
	running, failed, due := gui.operations.counts(gui.State.OperationsSeenAt, now)
	gui.operationCounts.Store(int64(running)<<32 | int64(failed))

	if !due.IsZero() && !gui.operationsRecount {
		gui.operationsRecount = true
		time.AfterFunc(due.Sub(now), func() {
			if gui.isStopped() {
				return
			}

			gui.g.Update(func(*gocui.Gui) error {
				gui.operationsRecount = false
				return gui.countOperations()
			})
		})
	}

	return gui.renderString(gui.g, "information", gui.getInformationContent())
}

// seeOperations is the operations on screen: their failures are seen.
func (gui *Gui) seeOperations() {
	gui.State.OperationsSeenAt = time.Now()

	if err := gui.countOperations(); err != nil {
		gui.Log.Error().Err(err).Send()
	}
}

// operationsStatusContent is the footer's operation counts.
func (gui *Gui) operationsStatusContent() string {
	counts := gui.operationCounts.Load()
	running, failed := int(counts>>32), int(counts&0xffffffff)

	var parts []string
	if failed > 0 {
		parts = append(parts, utils.ColoredString(fmt.Sprintf(gui.Tr.OperationsFailed, failed), color.FgRed))
	}

	if running > 0 {
		parts = append(parts, utils.ColoredString(fmt.Sprintf(gui.Tr.OperationsRunning, running), color.FgYellow))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ") + "  "
}

// operationCancel is `d`, asked first, then back to the list either way.
func (gui *Gui) operationCancel(operation *commands.Operation) error {
	back := func() error { return gui.openDaemon(daemonOperations, operation.Key()) }

	if operation.IsFinal() || !operation.Operation.MayCancel {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.OperationNotCancellable, operation.Operation.Description))
	}

	prompt := fmt.Sprintf(gui.Tr.ConfirmCancelOperation, operationLabel(operation))

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.CancellingStatus, func() error {
			if err := gui.IncusCommand.CancelOperation(operation); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refresh(back, gui.fetchOperations)
		})
	}, func(*gocui.Gui, *gocui.View) error { return back() })
}

// operationLabel names an operation for a prompt or the palette: what it
// does, and to what.
func operationLabel(operation *commands.Operation) string {
	if target := operation.Target(); target != "" {
		return operation.Operation.Description + " " + target
	}

	return operation.Operation.Description
}
