package tui

import tea "charm.land/bubbletea/v2"

type openedConfigMsg struct{ err error }

// openConfig is pkg/gui's handleOpenConfig; the opener may block, so it runs off the loop.
func (m *Model) openConfig() tea.Cmd {
	filename, open := m.cfg.ConfigFilename(), m.openFile

	return func() tea.Msg { return openedConfigMsg{err: open(filename)} }
}

func (m *Model) onOpenedConfig(msg openedConfigMsg) {
	if msg.err != nil {
		m.openError(msg.err.Error())
	}
}

// editConfig is pkg/gui's handleEditConfig: the editor, then the config read again.
func (m *Model) editConfig() tea.Cmd {
	cmd, err := m.os.EditFile(m.cfg.ConfigFilename())
	if err != nil {
		m.openError(err.Error())
		return nil
	}

	return m.runOnTerminal(cmd, nil, func(error) tea.Cmd {
		if err := m.reloadConfig(); err != nil {
			m.openError(err.Error())
		}

		return nil
	})
}
