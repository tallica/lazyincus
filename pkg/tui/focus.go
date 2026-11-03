package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// awayPollInterval: see docs/Panels.md, "What keeps them current".
const awayPollInterval = time.Minute

func (m *Model) awaySkips(last time.Time) bool {
	return m.away && time.Since(last) < awayPollInterval
}

func (m *Model) onBlur() {
	m.away = true
	m.cancelMain()
}

func (m *Model) onFocus() tea.Cmd {
	if !m.away {
		return nil
	}

	m.away = false
	m.mainTask.key = ""

	return m.refreshAll()
}
