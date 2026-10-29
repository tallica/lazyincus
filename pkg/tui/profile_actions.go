package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// defaultProfile is the profile every project has, and keeps.
const defaultProfile = "default"

// onProfile is pkg/gui's onSelected for the profiles panel.
func onProfile(action func(*Model, *commands.Profile) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		profile := m.selectedProfile()
		if profile == nil {
			return nil
		}

		return action(m, profile)
	}
}

func (m *Model) profileUsers(profile *commands.Profile) tea.Cmd {
	return m.showUsers(profile.Name, profile.IsUsedBy)
}

func (m *Model) profileEdit(profile *commands.Profile) tea.Cmd {
	return m.editInIncus(profile.Profile.Project, func() []fetch { return []fetch{m.fetchProfiles()} }, "profile", "edit", profile.Name)
}

// profileDelete is pkg/gui's: what the daemon would refuse - a default, one in use - it says rather than asks.
func (m *Model) profileDelete(profile *commands.Profile) tea.Cmd {
	if profile.Name == defaultProfile {
		m.openError(m.tr.CannotDeleteDefaultProfile)
		return nil
	}

	name := m.qualified(profile.Name, profile.Profile.Project, m.profilesSpan)
	if profile.UsedByCount() > 0 {
		m.refuseInUse(fmt.Sprintf(m.tr.ProfileNamed, name), profile.UsedByCount())
		return nil
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.DeleteProfile, name), func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, profile.Delete, m.reportOr(func() tea.Cmd {
			return refresh(m.fetchProfiles())
		}))
	})

	return nil
}

func (m *Model) profileCopy(profile *commands.Profile) tea.Cmd {
	m.copyMenu(copyValue{m.tr.CopyName, profile.Name})

	return nil
}
