package gui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
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
}

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
	}

	l.entries[operation.Key()] = &loggedOperation{operation: operation, seen: at}
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

// counts is how many operations are under way, and how many failed after
// since.
func (l *operationLog) counts(since time.Time) (running, failed int) {
	for _, logged := range l.entries {
		switch {
		case !logged.operation.IsFinal():
			running++
		case logged.operation.Operation.StatusCode == api.Failure && logged.seen.After(since):
			failed++
		}
	}

	return running, failed
}

func (l *operationLog) clear() {
	l.entries = nil
}

func (gui *Gui) getOperationsPanel() *panels.SideListPanel[*commands.Operation] {
	return &panels.SideListPanel[*commands.Operation]{
		ContextState: &panels.ContextState[*commands.Operation]{
			GetMainTabs: func() []panels.MainTab[*commands.Operation] {
				return []panels.MainTab[*commands.Operation]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderOperationInfo,
					},
				}
			},
			GetItemContextCacheKey: func(operation *commands.Operation) string {
				return "operations-" + operation.Key() + "-" + operation.Status() + "-" +
					operation.Operation.UpdatedAt.String() + "-" + operation.Progress()
			},
		},
		ListPanel: panels.ListPanel[*commands.Operation]{
			List: panels.NewFilteredList[*commands.Operation](),
			View: gui.Views.Operations,
		},
		NoItemsMessage: gui.Tr.NoOperations,
		Gui:            gui.intoInterface(),
		// Under way first, being what the list is for; then newest first.
		Sort: func(a, b *commands.Operation) bool {
			if a.IsFinal() != b.IsFinal() {
				return !a.IsFinal()
			}

			return a.Operation.CreatedAt.After(b.Operation.CreatedAt)
		},
		SameItem: func(a, b *commands.Operation) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(operation *commands.Operation) []string {
			return presentation.GetOperationDisplayStrings(operation, gui.State.SpansProjects.Operations)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index:    projectColumns(gui.State.SpansProjects.Operations) + 1,
				MinWidth: presentation.MinOperationDescriptionWidth,
			}}
		},
	}
}

func (gui *Gui) renderOperationInfo(operation *commands.Operation) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.operationInfoStr(operation) })
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
	output += line("Status", presentation.OperationStatus(operation))
	if op.Err != "" {
		output += line("Error", utils.ColoredString(op.Err, color.FgRed))
	}

	if !operation.IsFinal() {
		output += line("Progress", operation.Progress())
	}
	output += line("Class", op.Class)
	output += line("Started", op.CreatedAt.Local().Format(presentation.DateTimeFormat))

	if operation.IsFinal() {
		output += line("Took", presentation.OperationDuration(operation.Took()))
	} else {
		output += line("Updated", op.UpdatedAt.Local().Format(presentation.DateTimeFormat))
	}

	output += line("Cancellable", gui.yesNo(op.MayCancel && !operation.IsFinal()))
	// A daemon that isn't clustered says "none".
	if op.Location != "none" {
		output += line("Member", op.Location)
	}

	if resources := operation.Resources(); len(resources) > 0 {
		output += "\n" + gui.sectionHeading(gui.Tr.OperationResources) + "\n\n" + strings.Join(resources, "\n") + "\n"
	}

	// A websocket's metadata is its connection secrets.
	if op.Class == "task" && len(op.Metadata) > 0 {
		data, err := utils.MarshalIntoYaml(op.Metadata)
		if err == nil {
			output += "\n" + gui.sectionHeading(gui.Tr.OperationMetadata) + "\n\n" + utils.ColoredYamlString(string(data))
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

		gui.operations.reconcile(operations, asked, time.Now())

		return gui.showOperations()
	}, nil
}

func (gui *Gui) refreshOperationsQuiet() error {
	if err := gui.refresh(nil, gui.fetchOperations); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// recordOperation takes an operation event into the log. Off the main loop.
func (gui *Gui) recordOperation(operation *commands.Operation) {
	at := time.Now()

	gui.g.Update(func(*gocui.Gui) error {
		gui.operations.record(operation, at)

		return gui.showOperations()
	})
}

// showOperations lists the log and says in the footer what's running and
// what has failed unseen. Main loop only.
func (gui *Gui) showOperations() error {
	operations := gui.operations.list()

	gui.State.SpansProjects.Operations = spansMultipleProjects(
		lo.Map(operations, func(operation *commands.Operation, _ int) string { return operation.Project }))

	gui.Panels.Operations.SetItems(operations)

	if err := gui.Panels.Operations.RerenderList(); err != nil {
		return err
	}

	if gui.currentViewName() == "operations" {
		gui.State.OperationsSeenAt = time.Now()
	}

	return gui.countOperations()
}

// countOperations hands the footer its counts, which it reads off the main
// loop, and redraws it.
func (gui *Gui) countOperations() error {
	running, failed := gui.operations.counts(gui.State.OperationsSeenAt)
	gui.operationCounts.Store(int64(running)<<32 | int64(failed))

	return gui.renderString(gui.g, "information", gui.getInformationContent())
}

// seeOperations is the Operations tab getting focus: its failures are seen.
func (gui *Gui) seeOperations() {
	gui.State.OperationsSeenAt = time.Now()

	if err := gui.countOperations(); err != nil {
		gui.Log.Error(err)
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

// operationCancel is `d`.
func (gui *Gui) operationCancel(operation *commands.Operation) error {
	if operation.IsFinal() || !operation.Operation.MayCancel {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.OperationNotCancellable, operation.Operation.Description))
	}

	prompt := fmt.Sprintf(gui.Tr.ConfirmCancelOperation, operationLabel(operation))

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.CancellingStatus, func() error {
			if err := gui.IncusCommand.CancelOperation(operation); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refresh(nil, gui.fetchOperations)
		})
	}, nil)
}

// operationLabel names an operation for a prompt or the palette: what it
// does, and to what.
func operationLabel(operation *commands.Operation) string {
	if target := operation.Target(); target != "" {
		return operation.Operation.Description + " " + target
	}

	return operation.Operation.Description
}
