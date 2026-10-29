package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// onVolume is pkg/gui's onSelected for the volumes panel.
func onVolume(action func(*Model, *commands.Volume) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		volume := m.selectedVolume()
		if volume == nil {
			return nil
		}

		return action(m, volume)
	}
}

func (m *Model) volumeEdit(volume *commands.Volume) tea.Cmd {
	return m.editInIncus(volume.Volume.Project, func() []fetch { return []fetch{m.fetchVolumes()} },
		"storage", "volume", "edit", volume.Pool, volume.Volume.Type+"/"+volume.Name)
}

func (m *Model) volumeUsers(volume *commands.Volume) tea.Cmd {
	return m.showUsers(volume.Name, volume.IsUsedBy)
}

func (m *Model) volumeCopy(volume *commands.Volume) tea.Cmd {
	m.copyMenu(
		copyValue{m.tr.CopyName, volume.Name},
		copyValue{m.tr.CopyPool, volume.Pool},
	)

	return nil
}

// volumeDelete is pkg/gui's: a custom volume, and one nothing uses.
func (m *Model) volumeDelete(volume *commands.Volume) tea.Cmd {
	if !volume.IsCustom() {
		m.openError(m.tr.CannotDeleteManagedVolume)
		return nil
	}

	name := m.qualified(volume.Name, volume.Volume.Project, m.volumesSpan)
	if volume.UsedByCount() > 0 {
		m.refuseInUse(fmt.Sprintf(m.tr.VolumeNamed, name), volume.UsedByCount())
		return nil
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.DeleteVolume, name), func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, volume.Delete, m.reportOr(func() tea.Cmd {
			return refresh(m.fetchVolumes())
		}))
	})

	return nil
}

func (m *Model) refuseInUse(what string, users int) {
	m.openError(fmt.Sprintf(m.tr.CannotDeleteInUse, what, users))
}
