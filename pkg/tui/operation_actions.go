package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// showOperation: closing it goes back to the list.
func (m *Model) showOperation(operation *commands.Operation) tea.Cmd {
	back := m.backToDaemon(daemonOperations, operation.Key())
	m.openPopup(&popup{view: confirmationView, title: m.tr.OperationTitle, message: strings.TrimRight(m.operationInfo(operation), "\n")}, back, back)

	return nil
}

func (m *Model) operationInfo(operation *commands.Operation) string {
	op := operation.Operation
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", presentation.IdentityPadding) + value + "\n"
	}

	output := presentation.LocationLines(m.sessionLocation(operation.Project))
	output += line("Description", op.Description)
	output += line("Status", presentation.OperationStatus(operation, m.operations.retried(operation)))
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

	output += line("Cancellable", m.yesNo(op.MayCancel && !operation.IsFinal()))
	// A daemon that isn't clustered says "none".
	if op.Location != "none" {
		output += line("Member", op.Location)
	}

	if resources := operation.Resources(); len(resources) > 0 {
		output += "\n" + m.tr.OperationResources + ":\n" + strings.Join(resources, "\n") + "\n"
	}

	// A websocket's metadata is its connection secrets.
	if op.Class == "task" && len(op.Metadata) > 0 {
		data, err := utils.MarshalIntoYaml(op.Metadata)
		if err == nil {
			output += "\n" + m.tr.OperationMetadata + ":\n" + utils.ColoredYamlString(string(data))
		}
	}

	return output
}

// operationCancel: asked first, then back to the list either way.
func (m *Model) operationCancel(operation *commands.Operation) tea.Cmd {
	if operation.IsFinal() || !operation.Operation.MayCancel {
		m.openError(fmt.Sprintf(m.tr.OperationNotCancellable, operation.Operation.Description))
		return nil
	}

	back := m.backToDaemon(daemonOperations, operation.Key())
	prompt := fmt.Sprintf(m.tr.ConfirmCancelOperation, operationLabel(operation))
	m.openPopup(&popup{view: confirmationView, title: m.tr.Confirm, message: prompt, asks: true}, func() tea.Cmd {
		return m.waitingActionThen(m.tr.CancellingStatus, func() error { return m.incus.CancelOperation(operation) }, back, m.fetchOperations())
	}, back)

	return nil
}

// backToDaemon reopens W's popup on tab, on the row keyed selected.
func (m *Model) backToDaemon(tab daemonTab, selected string) func() tea.Cmd {
	return func() tea.Cmd {
		m.openDaemon(tab, selected)
		return nil
	}
}
