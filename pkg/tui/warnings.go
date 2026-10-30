package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

type warningsPollMsg struct{}

// warningsPollInterval is pkg/gui's: no event says the daemon has raised a warning.
const warningsPollInterval = 30 * time.Second

func (m *Model) fetchWarnings() fetch {
	ticket := m.refreshes.warnings.issue()

	return func() (func(), error) {
		warnings, err := m.incus.GetWarnings()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.warnings.admit(ticket) {
				m.showWarnings(warnings)
			}
		}, nil
	}
}

func (m *Model) showWarnings(warnings []*commands.Warning) {
	selected := m.daemonSelected(daemonWarnings)
	presentation.SortWarnings(warnings)
	m.warnings = warnings

	m.newWarnings = 0
	for _, warning := range warnings {
		if warning.IsNew() {
			m.newWarnings++
		}
	}

	m.redrawDaemon(daemonWarnings, selected)
}

func (m *Model) pollWarnings() tea.Cmd {
	return refreshQuiet(m.tick(warningsPollInterval, warningsPollMsg{}), m.fetchWarnings())
}

// warningsStatus is pkg/gui's warningsStatusContent.
func (m *Model) warningsStatus() string {
	if m.newWarnings == 0 {
		return ""
	}

	text := fmt.Sprintf(m.tr.WarningsCount, m.newWarnings)
	if m.newWarnings == 1 {
		text = m.tr.WarningCount
	}

	return utils.ColoredString(text, color.FgYellow) + "  "
}
