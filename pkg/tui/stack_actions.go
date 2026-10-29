package tui

import (
	"errors"
	"fmt"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

// onStackRow is pkg/gui's onSelected on Stacks: the key acts on the selected stack.
func onStackRow(action func(*Model, *commands.ComposeStack) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		stack := m.selectedStackRow()
		if stack == nil {
			return nil
		}

		return action(m, stack)
	}
}

// stackAdd is `a`: a directory to list, saved in state.yml with the remote it's pinned to.
func (m *Model) stackAdd() tea.Cmd {
	m.openTextPrompt(m.tr.AddStackPrompt, m.tr.AddStackHint, "", func(m *Model, input string) tea.Cmd {
		return m.saveStackThen(m.tr.AddingStackStatus, input, "")
	})

	return nil
}

// stackEdit is `e`: the stack's entry in the add prompt, saved in place so
// the list keeps its order in state.yml.
func (m *Model) stackEdit(stack *commands.ComposeStack) tea.Cmd {
	if !stack.Saved {
		m.openError(fmt.Sprintf(m.tr.CannotEditLocalStack, presentation.StackPath(stack, m.home)))
		return nil
	}

	initial := commands.StackRef(stack.Remote, commands.ShortenHome(stack.Dir, m.home))
	m.openTextPrompt(m.tr.EditStackPrompt, m.tr.EditStackHint, initial, func(m *Model, input string) tea.Cmd {
		return m.saveStackThen(m.tr.SavingStatus, input, stack.Ref())
	})

	return nil
}

func (m *Model) saveStackThen(status, input, replacing string) tea.Cmd {
	var ref string

	return m.withWaitingStatusThen(status, func() error {
		var err error

		ref, err = m.saveStack(input, replacing)

		return err
	}, m.reportOr(func() tea.Cmd {
		if ref == "" {
			return nil
		}

		return refresh(m.fetchStacks(), func() (func(), error) {
			return func() { m.selectStack(ref) }, nil
		})
	}))
}

// saveStack takes a directory only once incus-compose has read a compose
// project from it, and returns the ref saved, "" when nothing changed. Off the loop.
func (m *Model) saveStack(input, replacing string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	remote, path := commands.SplitStackInput(input, func(name string) bool {
		return name == m.incus.RemoteName() || m.remotes.known(name)
	})
	// Pinned to where it was added, so a session on another remote never runs its verbs there.
	if remote == "" {
		remote = m.incus.RemoteName()
	}

	dir, err := commands.ResolveStackDir(path, cwd, m.home)
	if err != nil {
		return "", err
	}

	ref := commands.StackRef(remote, dir)
	if ref == replacing {
		return "", nil
	}

	if err := commands.CheckStackDir(dir); err != nil {
		return "", err
	}

	state, err := m.cfg.LoadAppState()
	if err != nil {
		return "", err
	}

	if slices.Contains(state.Stacks, ref) || (m.onSessionRemote(remote) && m.isLocalStack(dir)) {
		return "", fmt.Errorf(m.tr.StackAlreadyListed, ref)
	}

	stack := m.loadStack(dir)
	if stack.Err != nil {
		return "", fmt.Errorf(m.tr.StackNotComposeProject, dir, stack.Err)
	}

	save := m.cfg.AddStack
	if replacing != "" {
		save = func(ref string) error { return m.cfg.ReplaceStack(replacing, ref) }
	}

	if err := save(ref); err != nil {
		if errors.Is(err, config.ErrStackListed) {
			return "", fmt.Errorf(m.tr.StackAlreadyListed, ref)
		}

		return "", err
	}

	m.stackCache.put(stack)

	return ref, nil
}

// isLocalStack is whether dir is the local stack's, listed.
func (m *Model) isLocalStack(dir string) bool {
	if dir != m.localStackDir {
		return false
	}

	return m.localStackExplicit || m.stackCache.get(dir, false, m.loadStack).Err == nil
}

// selectStack moves the cursor to ref's row, and the focus to Stacks.
func (m *Model) selectStack(ref string) {
	p := m.panelNamed("stacks")

	row := slices.IndexFunc(p.shown, func(item int) bool { return m.stacks[item].Ref() == ref })
	if row < 0 {
		return
	}

	p.selectRow(row, m.listRows(p))
	m.focusView("stacks")
}

// stackEditCompose is `c`: the compose file in the user's editor, read again once it closes.
func (m *Model) stackEditCompose(stack *commands.ComposeStack) tea.Cmd {
	file, err := commands.ComposeFile(stack.Dir)
	if err != nil {
		m.openError(err.Error())
		return nil
	}

	cmd, err := m.os.EditFile(file)
	if err != nil {
		m.openError(err.Error())
		return nil
	}

	return m.runSubprocess(cmd, "", func() tea.Cmd {
		m.stackCache.forget(stack.Dir)

		return refresh(m.fetchStacks(), m.fetchServices())
	})
}

// stackRemove is `D`: forget a saved stack, after asking. Nothing on the
// daemon changes, and the local stack isn't saved to be removed.
func (m *Model) stackRemove(stack *commands.ComposeStack) tea.Cmd {
	path := presentation.StackPath(stack, m.home)

	if !stack.Saved {
		m.openError(fmt.Sprintf(m.tr.CannotRemoveLocalStack, path))
		return nil
	}

	// Also the local stack, it keeps its row: only the saved entry goes.
	prompt := m.tr.ConfirmRemoveStack
	if stack.Local {
		prompt = m.tr.ConfirmRemoveSavedLocalStack
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(prompt, stack.Title(), path), func() tea.Cmd {
		cfg, ref := m.cfg, stack.Ref()

		return m.withWaitingStatusThen(m.tr.RemovingStatus, func() error { return cfg.RemoveStack(ref) },
			m.reportOr(func() tea.Cmd { return refresh(m.fetchStacks()) }))
	})

	return nil
}

// stackCopy offers each published port at its address, named for the
// instance and the port inside it; a port whose host isn't known has none.
func (m *Model) stackCopy(stack *commands.ComposeStack) tea.Cmd {
	values := []copyValue{
		{m.tr.CopyProject, stack.Name},
		{m.tr.CopyDirectory, stack.Dir},
	}

	if state, _ := m.composeState.load(); state.IsOf(stack) {
		for _, service := range presentation.SortedServices(state.Services) {
			for _, instance := range service.SortedInstances() {
				for _, port := range instance.Latest().PublishedPorts(m.publishHostFor(stack.Remote)) {
					label := presentation.EndpointName(service, instance)
					if port.Target != "" {
						label += " → " + port.Target
					}

					if port.Protocol != "tcp" {
						label += "/" + port.Protocol
					}

					values = append(values, copyValue{label, port.Address()})
				}
			}
		}
	}

	m.copyMenu(values...)

	return nil
}

// stackSwitchRemote is space on Stacks: the rest of the screen to the
// stack's remote, the way `R` would. One already there has nowhere to go.
func (m *Model) stackSwitchRemote(stack *commands.ComposeStack) tea.Cmd {
	if m.onSessionRemote(stack.Remote) {
		return nil
	}

	return m.stackSwitchRemoteThen(stack, func() tea.Cmd { return nil })
}

// stackSwitchRemoteThen is stackSwitchRemote with then run once the panels
// are on the stack's remote, or straight away when they already are.
func (m *Model) stackSwitchRemoteThen(stack *commands.ComposeStack, then func() tea.Cmd) tea.Cmd {
	if m.onSessionRemote(stack.Remote) {
		return then()
	}

	if !m.remotes.known(stack.Remote) {
		m.openError(m.unknownRemote(stack.Remote).Error())
		return nil
	}

	return m.switchToRemoteThen(stack.Remote, then)
}
