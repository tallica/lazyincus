package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// tagBumpedMsg is the compose file edited for bumps, or why it wasn't.
type tagBumpedMsg struct {
	target   composeTarget
	services []string
	err      error
}

// bumpTags is `T`: move services to beacon's newer tags, and recreate them.
func bumpTags(m *Model, stack *commands.ComposeStack) tea.Cmd {
	var bumps []commands.ComposeTagBump

	for _, service := range m.stackServiceList(stack) {
		if bump, ok := service.TagBump(); ok {
			bumps = append(bumps, bump)
		}
	}

	if len(bumps) == 0 {
		return m.withTransientStatus(fmt.Sprintf(m.tr.NoNewerTags, stack.Name), 3*time.Second)
	}

	items := make([]*menuItem, 0, len(bumps))
	for _, bump := range bumps {
		items = append(items, &menuItem{labelColumns: []string{bump.Service, bump.From, "→ " + bump.To}, checked: true})
	}

	m.openMenu(menuOptions{
		title: m.tr.ComposeBumpTagsTitle,
		items: items,
		onConfirm: func(picked []*menuItem) tea.Cmd {
			chosen := make([]commands.ComposeTagBump, 0, len(picked))
			for _, item := range picked {
				for _, bump := range bumps {
					if bump.Service == item.labelColumns[0] {
						chosen = append(chosen, bump)
					}
				}
			}

			return m.confirmTagBumps(stack, chosen)
		},
	})

	return nil
}

func (m *Model) confirmTagBumps(stack *commands.ComposeStack, bumps []commands.ComposeTagBump) tea.Cmd {
	target := stackTarget(stack)

	moves := make([]string, 0, len(bumps))
	services := make([]string, 0, len(bumps))
	for _, bump := range bumps {
		moves = append(moves, bump.Service+" to "+bump.To)
		services = append(services, bump.Service)
	}

	message := fmt.Sprintf(m.tr.ConfirmComposeBumpTags, m.onRemote(strings.Join(moves, ", "), target.remote))

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

// onTagBumped recreates the moved services; --no-deps, or up recreates their dependencies too.
func (m *Model) onTagBumped(msg tagBumpedMsg) tea.Cmd {
	m.stackCache.forget(msg.target.dir)

	if msg.err != nil {
		m.openError(fmt.Sprintf(m.tr.ComposeBumpTagsFailed, msg.err))
		return nil
	}

	args := append([]string{"up", "--recreate", "--pull", "missing", "--no-deps", "--detach"}, msg.services...)

	return m.composeRun(msg.target, args...)
}
