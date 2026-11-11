package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// tagBumpedMsg is the compose file edited for bumps, or why it wasn't.
type tagBumpedMsg struct {
	target   composeTarget
	services []string
	err      error
}

// confirmTagBumps can't be skipped with a: it edits the compose file.
func (m *Model) confirmTagBumps(target composeTarget, bumps []commands.ComposeTagBump, services []string) tea.Cmd {
	moves := make([]string, 0, len(bumps))
	for _, bump := range bumps {
		moves = append(moves, bump.Service+" to "+bump.To)
	}

	message := fmt.Sprintf(m.tr.ConfirmComposeBumpTags, strings.Join(moves, ", "),
		m.onRemote(strings.Join(services, ", "), target.remote))

	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return func() tea.Msg {
			file, err := commands.ComposeFile(target.dir)
			if err == nil {
				err = commands.RetagComposeImages(file, bumps)
			}

			return tagBumpedMsg{target: target, services: services, err: err}
		}
	})

	return nil
}

func (m *Model) onTagBumped(msg tagBumpedMsg) tea.Cmd {
	m.stackCache.forget(msg.target.dir)

	if msg.err != nil {
		m.openError(fmt.Sprintf(m.tr.ComposeBumpTagsFailed, msg.err))
		return nil
	}

	return m.recreateOnPull(msg.target, msg.services)
}
