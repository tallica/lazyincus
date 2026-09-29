package gui

import (
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
)

// onStack binds a key to a compose verb on the Stacks panel's selection:
// the Services panel's verbs with the SERVICE argument left off. A stack
// whose config couldn't be read says why instead.
func (gui *Gui) onStack(action func(*commands.ComposeStack) error) func(*gocui.Gui, *gocui.View) error {
	return onSelected(gui.Panels.Stacks, func(stack *commands.ComposeStack) error {
		if stack.Err != nil {
			return gui.createErrorPanel(stack.Err.Error())
		}

		return action(stack)
	})
}

// onStackTarget is onStack for a verb that needs only the target.
func (gui *Gui) onStackTarget(action func(composeTarget) error) func(*gocui.Gui, *gocui.View) error {
	return gui.onStack(func(stack *commands.ComposeStack) error { return action(stackTarget(stack)) })
}

// composeVerb is a verb run as it is, confirm naming the prompt it asks
// first, if any.
func (gui *Gui) composeVerb(confirm string, args ...string) func(composeTarget) error {
	return func(target composeTarget) error {
		if confirm != "" {
			return gui.composeConfirm(confirm, target, args...)
		}

		return gui.composeRun(target, args...)
	}
}

// stackPause is `p` on a stack: frozen throughout thaws, anything else
// freezes, each service voting.
func (gui *Gui) stackPause(stack *commands.ComposeStack) error {
	if stack.Status() == commands.ServiceNone {
		return gui.createErrorPanel(gui.Tr.StackNotRunning)
	}

	return gui.composeRun(stackTarget(stack), composePauseVerb(stack.ServiceStatuses()...))
}
