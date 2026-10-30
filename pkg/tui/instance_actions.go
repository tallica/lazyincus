package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

func onInstance(action func(*Model, *commands.Instance) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		instance := m.selectedInstance()
		if instance == nil {
			return nil
		}

		return action(m, instance)
	}
}

// qualified: the project named when the list holds several.
func (m *Model) qualified(name, project string, spans bool) string {
	if !spans || project == "" {
		return name
	}

	return fmt.Sprintf(m.tr.InProject, name, project)
}

func (m *Model) qualifiedInstance(instance *commands.Instance) string {
	return m.onRemote(m.qualified(instance.Name, instance.Project, m.showProject), instance.Remote)
}

// inTransition: the row's mark comes off once the refresh after action lands.
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

// instancePauseResume: pause or resume, whichever the state calls for.
func (m *Model) instancePauseResume(instance *commands.Instance) tea.Cmd {
	if instance.Instance.Status == "Frozen" {
		return m.inTransition(m.tr.ResumingStatus, instance, "Unfreezing", instance.Unfreeze)
	}

	return m.inTransition(m.tr.PausingStatus, instance, "Freezing", instance.Freeze)
}

// instanceDelete: offering to force it when Incus refuses a running one.
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

// instanceCmd: `incus` on the instance's remote and project.
func (m *Model) instanceCmd(instance *commands.Instance, args ...string) *exec.Cmd {
	cmd := m.os.NewCmd("incus", append(projectCLIArgs(instance.Project), args...)...)

	return commands.WithRemote(cmd, instance.Remote)
}

func projectCLIArgs(project string) []string {
	if project == "" {
		return nil
	}

	return []string{"--project", project}
}

// instanceEdit: `incus config edit`, then the lists it changed.
func (m *Model) instanceEdit(instance *commands.Instance) tea.Cmd {
	return m.runSubprocess(m.instanceCmd(instance, "config", "edit", instance.Name), "", func() tea.Cmd {
		return refreshQuiet(nil, m.fetchInstances(), m.fetchServices())
	})
}

func (m *Model) instanceEditFile(instance *commands.Instance) tea.Cmd {
	m.openTextPrompt(fmt.Sprintf(m.tr.EditFilePrompt, instance.Name), m.tr.EditFileHint, "/", func(m *Model, path string) tea.Cmd {
		return m.runSubprocess(m.instanceFileEditCmd(instance, path), "", nil)
	})

	return nil
}

// instanceFileEditCmd takes a path without its leading slash as meaning one.
func (m *Model) instanceFileEditCmd(instance *commands.Instance, path string) *exec.Cmd {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return m.instanceCmd(instance, "file", "edit", instance.Name+path)
}

// instanceRename: a running instance stopped first, once confirmed.
func (m *Model) instanceRename(instance *commands.Instance) tea.Cmd {
	m.openTextPrompt(fmt.Sprintf(m.tr.RenameInstancePrompt, instance.Name), m.tr.RenameHint, instance.Name, func(m *Model, name string) tea.Cmd {
		if name == instance.Name {
			return nil
		}

		if isStopped(instance.Latest()) {
			return m.withWaitingStatusThen(m.tr.RenamingStatus, func() error { return instance.Rename(name, false) },
				m.afterRename(instance, name, func() {}))
		}

		message := fmt.Sprintf(m.tr.StopToRename, m.qualifiedInstance(instance), name)
		m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
			end := instance.BeginTransition("Stopping")
			m.rerenderInstances()

			return m.withWaitingStatusThen(m.tr.RenamingStatus, func() error {
				if err := instance.Stop(); err != nil {
					return err
				}

				return instance.Rename(name, true)
			}, m.afterRename(instance, name, end))
		})

		return nil
	})

	return nil
}

// afterRename moves the cursor to where the new name sorts once it's listed.
func (m *Model) afterRename(instance *commands.Instance, name string, end func()) func(error) tea.Cmd {
	return func(err error) tea.Cmd {
		if err != nil {
			end()
			m.rerenderInstances()
			m.openError(err.Error())
			return nil
		}

		key := (&commands.Instance{Name: name, Project: instance.Project}).Key()
		fetches := append(m.instanceFetches(), func() (func(), error) {
			return func() { m.selectInstance(key) }, nil
		})

		return refreshEnding([]func(){end}, fetches...)
	}
}

// instanceEditDescription: one line, a written one's lines run together.
func (m *Model) instanceEditDescription(instance *commands.Instance) tea.Cmd {
	current := strings.Join(strings.Fields(instance.Latest().Instance.Description), " ")
	title := fmt.Sprintf(m.tr.EditDescriptionPrompt, instance.Name)

	m.textPrompt(title, m.tr.EditDescriptionHint, current, acceptEmpty, func(m *Model, description string) tea.Cmd {
		if description == current {
			return nil
		}

		return m.withWaitingStatusThen(m.tr.SavingStatus, func() error { return instance.SetDescription(description) },
			func(err error) tea.Cmd {
				if err != nil {
					m.openError(err.Error())
					return nil
				}

				return refresh(m.fetchInstances(), m.fetchServices())
			})
	})

	return nil
}

// instanceAttachConsole: `incus console`, which prints its own detach hint.
func (m *Model) instanceAttachConsole(instance *commands.Instance) tea.Cmd {
	if !instance.IsRunning() {
		m.openError(m.tr.CannotAttachStoppedInstanceError)
		return nil
	}

	// `incus console` on one of these fails with the daemon's own "operation
	// not supported by device".
	if instance.IsOCI() {
		m.openError(m.tr.CannotAttachAppContainerError)
		return nil
	}

	return m.runSubprocess(m.instanceCmd(instance, "console", instance.Name), "", nil)
}

// instanceExecShell: the CLI's exec, not the client's websocket.
func (m *Model) instanceExecShell(instance *commands.Instance) tea.Cmd {
	if !instance.IsRunning() {
		m.openError(m.tr.CannotExecStoppedInstanceError)
		return nil
	}

	cmd := m.instanceCmd(instance, "exec", instance.Name, "--", "sh", "-c",
		"exec $(command -v bash || command -v ash || command -v sh)")

	return m.runSubprocess(cmd, m.tr.ExitShellToReturn, nil)
}
