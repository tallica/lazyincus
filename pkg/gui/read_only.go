package gui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

// guardReadOnly makes binding refuse, naming why, when the remote it would
// change is read-only.
func (gui *Gui) guardReadOnly(binding *Binding) {
	handler := binding.Handler
	binding.Handler = func(g *gocui.Gui, v *gocui.View) error {
		if gui.Config.ReadOnly {
			return gui.createErrorPanel(gui.Tr.ReadOnlySession)
		}

		if remote := gui.actionRemote(binding.ViewName); gui.isReadOnly(remote) {
			return gui.createErrorPanel(fmt.Sprintf(gui.Tr.ReadOnlyRemote, remote))
		}

		return handler(g, v)
	}
}

// actionRemote is the remote a key on view acts on: the selected stack's on
// Stacks and Services, the selected snapshot's instance's on Snapshots -
// either can be another remote's - and otherwise the session's.
func (gui *Gui) actionRemote(view string) string {
	switch view {
	case "stacks", "services":
		if stack := gui.selectedStack.Load(); stack != nil && stack.Remote != "" {
			return stack.Remote
		}
	case "snapshots":
		if snapshot, err := gui.Panels.Snapshots.GetSelectedItem(); err == nil && snapshot.Instance != nil && snapshot.Instance.Remote != "" {
			return snapshot.Instance.Remote
		}
	}

	return gui.IncusCommand.RemoteName()
}

func (gui *Gui) isReadOnly(remote string) bool {
	return gui.Config.ReadOnly || gui.Config.UserConfig.Remotes[remote].ReadOnly
}
