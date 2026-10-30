package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

type operationsRecountMsg struct{}

func (m *Model) fetchOperations() fetch {
	ticket := m.refreshes.operations.issue()

	return func() (func(), error) {
		asked := time.Now()

		operations, err := m.incus.GetOperations()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.operations.admit(ticket) {
				selected := m.daemonSelected(daemonOperations)
				m.operations.reconcile(operations, asked, time.Now())
				m.showOperations(selected)
			}
		}, nil
	}
}

// recordOperation takes an operation event into the log; a closed stream's never reaches it.
func (m *Model) recordOperation(operation *commands.Operation) {
	selected := m.daemonSelected(daemonOperations)
	m.operations.record(operation, time.Now())
	m.showOperations(selected)
}

// countOperations recounts the footer's operations, again once the next running one has lasted.
func (m *Model) countOperations() {
	now := time.Now()
	running, failed, due := m.operations.counts(m.operationsSeenAt, now)
	m.operationCounts = [2]int{running, failed}

	if !due.IsZero() && !m.operationsRecount {
		m.operationsRecount = true
		m.queued = append(m.queued, m.tick(due.Sub(now), operationsRecountMsg{}))
	}
}

func (m *Model) onOperationsRecount() {
	m.operationsRecount = false
	m.countOperations()
}

// operationsStatus is pkg/gui's operationsStatusContent.
func (m *Model) operationsStatus() string {
	running, failed := m.operationCounts[0], m.operationCounts[1]

	var parts []string
	if failed > 0 {
		parts = append(parts, utils.ColoredString(fmt.Sprintf(m.tr.OperationsFailed, failed), color.FgRed))
	}

	if running > 0 {
		parts = append(parts, utils.ColoredString(fmt.Sprintf(m.tr.OperationsRunning, running), color.FgYellow))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ") + "  "
}
