package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

// showWarning: closing it goes back to the list.
func (m *Model) showWarning(warning *commands.Warning) tea.Cmd {
	back := m.backToDaemon(daemonWarnings, warning.Key())
	m.openPopup(&popup{view: confirmationView, title: m.tr.WarningTitle, message: m.warningDetails(warning)}, back, back)

	return nil
}

func (m *Model) warningDetails(warning *commands.Warning) string {
	w := warning.Warning
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", presentation.IdentityPadding) + value + "\n"
	}

	project := w.Project
	if project == "" {
		project = m.tr.WarningServerWide
	}

	details := line("Type", w.Type) +
		line("Message", w.LastMessage) +
		line("Severity", w.Severity) +
		line("Status", w.Status) +
		line("Project", project) +
		line("Entity", warning.Entity()) +
		line("Count", fmt.Sprint(w.Count)) +
		line("First seen", w.FirstSeenAt.Local().Format(presentation.SecondsFormat)) +
		line("Last seen", w.LastSeenAt.Local().Format(presentation.SecondsFormat))

	if w.Location != "" && w.Location != "none" {
		details += line("Member", w.Location)
	}

	return strings.TrimRight(details, "\n")
}

// acknowledgeWarning is `a`; the popup stays open, redrawn.
func (m *Model) acknowledgeWarning(warning *commands.Warning) tea.Cmd {
	return m.waitingActionThen(m.tr.AcknowledgingStatus, func() error { return m.incus.AcknowledgeWarning(warning) }, nil, m.fetchWarnings())
}

// deleteWarning's prompt shows all of it: two of a type can differ only in count and first sighting.
func (m *Model) deleteWarning(warning *commands.Warning) tea.Cmd {
	prompt := m.warningDetails(warning) + "\n\n" + m.tr.ConfirmDeleteWarning
	m.openPopup(&popup{view: confirmationView, title: m.tr.Confirm, message: prompt, asks: true}, func() tea.Cmd {
		return m.waitingActionThen(m.tr.RemovingStatus, func() error { return m.incus.DeleteWarning(warning) },
			m.backToDaemon(daemonWarnings, ""), m.fetchWarnings())
	}, m.backToDaemon(daemonWarnings, warning.Key()))

	return nil
}
