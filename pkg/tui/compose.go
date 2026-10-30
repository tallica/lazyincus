package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// composeTarget is pkg/gui's: a stack, narrowed to one service unless service is empty.
type composeTarget struct {
	remote  string
	dir     string
	project string
	service string
}

func stackTarget(stack *commands.ComposeStack) composeTarget {
	return composeTarget{remote: stack.Remote, dir: stack.Dir, project: stack.Name}
}

// onStack is onStackRow with pkg/gui's withStack checks: a broken stack says why.
func onStack(action func(*Model, *commands.ComposeStack) tea.Cmd) func(*Model) tea.Cmd {
	return onStackRow(func(m *Model, stack *commands.ComposeStack) tea.Cmd {
		if stack.Err != nil {
			m.openError(stack.Err.Error())
			return nil
		}

		if !m.onSessionRemote(stack.Remote) && !m.remotes.known(stack.Remote) {
			m.openError(m.unknownRemote(stack.Remote).Error())
			return nil
		}

		return action(m, stack)
	})
}

// onStackTarget is onStack for a verb that needs only the target.
func onStackTarget(action func(*Model, composeTarget) tea.Cmd) func(*Model) tea.Cmd {
	return onStack(func(m *Model, stack *commands.ComposeStack) tea.Cmd { return action(m, stackTarget(stack)) })
}

// composeCmd is composeRun's command: the arguments, then the service, the remote by name alone.
func (m *Model) composeCmd(target composeTarget, args ...string) *exec.Cmd {
	if target.service != "" {
		args = append(args, target.service)
	}

	return commands.WithRemote(m.incus.ComposeCmd(target.dir, args...), target.remote)
}

// composeRun is pkg/gui's.
func (m *Model) composeRun(target composeTarget, args ...string) tea.Cmd {
	return m.composeRunThen(target, args, nil)
}

// composeRunThen is composeRun with after run alongside the refresh it ends in.
func (m *Model) composeRunThen(target composeTarget, args []string, after func() tea.Cmd) tea.Cmd {
	return m.runWatched(m.composeCmd(target, args...), target.remote, target.project, func(failures []daemonFailure) tea.Cmd {
		m.stackCache.forget(target.dir)
		m.showFailures(failures, target.remote)

		next := refresh(m.fetchInstances(), m.fetchStacks(), m.fetchServices())
		if after == nil {
			return next
		}

		return tea.Batch(next, after())
	})
}

// composeTargetName names what a confirmation is about, the service or the project.
func (m *Model) composeTargetName(target composeTarget) string {
	if target.service != "" {
		return m.onRemote(fmt.Sprintf(m.tr.ComposeTargetService, target.service), target.remote)
	}

	return m.onRemote(fmt.Sprintf(m.tr.ComposeTargetProject, target.project), target.remote)
}

func (m *Model) composeConfirm(prompt string, target composeTarget, args ...string) tea.Cmd {
	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(prompt, m.composeTargetName(target)), func() tea.Cmd {
		return m.composeRun(target, args...)
	})

	return nil
}

// composeConfirmOnce is composeConfirm with `a` to stop asking, kept under id.
func (m *Model) composeConfirmOnce(id, prompt string, target composeTarget, args ...string) tea.Cmd {
	return m.openSkippableConfirmation(id, m.tr.Confirm, fmt.Sprintf(prompt, m.composeTargetName(target)), func() tea.Cmd {
		return m.composeRun(target, args...)
	})
}

// composeVerb is a verb run as it is, confirm naming the prompt it asks first, if any.
func composeVerb(confirm string, args ...string) func(*Model, composeTarget) tea.Cmd {
	return func(m *Model, target composeTarget) tea.Cmd {
		if confirm != "" {
			return m.composeConfirm(confirm, target, args...)
		}

		return m.composeRun(target, args...)
	}
}

// composeUp is `up --detach`: attached, `up` would tail every service's logs.
func composeUp(m *Model, target composeTarget) tea.Cmd {
	return m.composeRun(target, "up", "--detach")
}

func composeRecreateArgs(pull string) []string {
	return []string{"up", "--recreate", "--pull", pull, "--detach"}
}

// composeRecreateMenu is `U`: `up --recreate`, with or without pulling first.
func composeRecreateMenu(m *Model, target composeTarget) tea.Cmd {
	m.openMenu(menuOptions{
		title: m.tr.ComposeRecreateMenuTitle,
		items: []*menuItem{
			{label: m.tr.ComposeRecreate, onPress: func(m *Model) tea.Cmd {
				return m.composeConfirmOnce("recreate", m.tr.ConfirmComposeRecreate, target, composeRecreateArgs("never")...)
			}},
			{label: m.tr.ComposeRecreatePullOption, onPress: func(m *Model) tea.Cmd {
				return m.composeConfirmOnce("recreate-pull", m.tr.ConfirmComposeRecreatePull, target, composeRecreateArgs("always")...)
			}},
		},
	})

	return nil
}

func composeDownMenu(m *Model, target composeTarget) tea.Cmd {
	m.openMenu(menuOptions{
		title: m.tr.ComposeDownMenuTitle,
		items: []*menuItem{
			{label: m.tr.ComposeDownOption, onPress: func(m *Model) tea.Cmd {
				return m.composeConfirm(m.tr.ConfirmComposeDown, target, "down")
			}},
			{label: m.tr.ComposeDownWithVolumesOption, onPress: func(m *Model) tea.Cmd {
				return m.composeConfirm(m.tr.ConfirmComposeDownWithVolumes, target, "down", "--volumes")
			}},
		},
	})

	return nil
}

// composePauseVerb: frozen throughout thaws, anything else freezes; idle services don't vote.
func composePauseVerb(statuses ...string) string {
	frozen := false

	for _, status := range statuses {
		if status == commands.ServiceNone {
			continue
		}

		if !strings.EqualFold(status, "Frozen") {
			return "pause"
		}

		frozen = true
	}

	if frozen {
		return "unpause"
	}

	return "pause"
}

// stackPause is `p` on a stack, each service voting.
func (m *Model) stackPause(stack *commands.ComposeStack) tea.Cmd {
	switch {
	case stack.StatusErr != nil:
		m.openError(stack.StatusErr.Error())
	case stack.StatusPending:
		m.openError(fmt.Sprintf(m.tr.ConnectingTo, stack.Remote))
	case stack.Status() == commands.ServiceNone:
		m.openError(m.tr.StackNotRunning)
	default:
		return m.composeRun(stackTarget(stack), composePauseVerb(stack.ServiceStatuses()...))
	}

	return nil
}
