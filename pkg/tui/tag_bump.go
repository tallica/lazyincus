package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// tagBumpedMsg is the compose file edited for bumps, or why it wasn't.
type tagBumpedMsg struct {
	target   composeTarget
	services []string
	recheck  bool
	err      error
}

// confirmTagBumps can't be skipped with a: it edits the compose file.
func (m *Model) confirmTagBumps(target composeTarget, bumps []commands.ComposeTagBump, services []string, recheck bool) tea.Cmd {
	moves := make([]string, 0, len(bumps))
	for _, bump := range bumps {
		moves = append(moves, bump.Service+" to "+bump.To)
	}

	message := fmt.Sprintf(m.tr.ConfirmComposeBumpTags, strings.Join(moves, ", "),
		m.onRemote(strings.Join(services, ", "), target.remote))

	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return func() tea.Msg {
			var made []commands.ComposeTagBump

			file, err := commands.ComposeFile(target.dir)
			if err == nil {
				made, err = commands.RetagComposeImages(file, bumps)
			}

			// A move not made leaves its service out; the rest still go.
			kept := slices.DeleteFunc(slices.Clone(services), func(service string) bool {
				moved := func(bump commands.ComposeTagBump) bool { return bump.Service == service }
				return slices.ContainsFunc(bumps, moved) && !slices.ContainsFunc(made, moved)
			})

			return tagBumpedMsg{target: target, services: kept, recheck: recheck, err: err}
		}
	})

	return nil
}

func (m *Model) onTagBumped(msg tagBumpedMsg) tea.Cmd {
	m.stackCache.forget(msg.target.dir)

	if msg.err != nil {
		m.openError(fmt.Sprintf(m.tr.ComposeBumpTagsFailed, msg.err))
	}

	if len(msg.services) == 0 {
		return nil
	}

	return m.recreateOnPull(msg.target, msg.services, msg.recheck)
}
