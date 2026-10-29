package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// onInstance is pkg/gui's onSelected for the instances panel.
func onInstance(action func(*Model, *commands.Instance) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		instance := m.selectedInstance()
		if instance == nil {
			return nil
		}

		return action(m, instance)
	}
}

// qualified is pkg/gui's: the project named when the list holds several.
func (m *Model) qualified(name, project string, spans bool) string {
	if !spans || project == "" {
		return name
	}

	return fmt.Sprintf(m.tr.InProject, name, project)
}

func (m *Model) qualifiedInstance(instance *commands.Instance) string {
	return m.onRemote(m.qualified(instance.Name, instance.Project, m.showProject), instance.Remote)
}

// inTransition is pkg/gui's: the row's mark comes off once the refresh after action lands.
func (m *Model) inTransition(waiting string, instance *commands.Instance, status string, action func() error) tea.Cmd {
	end := instance.BeginTransition(status)
	m.rerenderInstances()

	return m.withWaitingStatusThen(waiting, action, func(err error) tea.Cmd {
		if err != nil {
			end()
			m.rerenderInstances()
			m.openError(err.Error())
			return nil
		}

		return refreshEnding([]func(){end}, m.instanceFetches()...)
	})
}

func (m *Model) instanceStart(instance *commands.Instance) tea.Cmd {
	return m.inTransition(m.tr.StartingStatus, instance, "Starting", instance.Start)
}

func (m *Model) instanceStop(instance *commands.Instance) tea.Cmd {
	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.StopInstance, m.qualifiedInstance(instance)), func() tea.Cmd {
		return m.inTransition(m.tr.StoppingStatus, instance, "Stopping", instance.Stop)
	})

	return nil
}

func (m *Model) instanceRestart(instance *commands.Instance) tea.Cmd {
	return m.inTransition(m.tr.RestartingStatus, instance, "Restarting", instance.Restart)
}

// instancePauseResume is pkg/gui's: pause or resume, whichever the state calls for.
func (m *Model) instancePauseResume(instance *commands.Instance) tea.Cmd {
	if instance.Instance.Status == "Frozen" {
		return m.inTransition(m.tr.ResumingStatus, instance, "Unfreezing", instance.Unfreeze)
	}

	return m.inTransition(m.tr.PausingStatus, instance, "Freezing", instance.Freeze)
}

// instanceDelete is pkg/gui's, offering to force it when Incus refuses a running one.
func (m *Model) instanceDelete(instance *commands.Instance) tea.Cmd {
	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.DeleteInstance, m.qualifiedInstance(instance)), func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RemovingStatus, instance.Delete, func(err error) tea.Cmd {
			switch {
			case errors.Is(err, commands.ErrInstanceRunning):
				m.promptToForceDeleteInstance(instance)
				return nil
			case err != nil:
				m.openError(err.Error())
				return nil
			}

			return refresh(m.instanceFetches()...)
		})
	})

	return nil
}

func (m *Model) promptToForceDeleteInstance(instance *commands.Instance) {
	m.openConfirmation(m.tr.Confirm, m.tr.MustForceToRemove, func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.ForceRemovingStatus, instance.ForceDelete, func(err error) tea.Cmd {
			if err != nil {
				m.openError(err.Error())
				return nil
			}

			return refresh(m.instanceFetches()...)
		})
	})
}
