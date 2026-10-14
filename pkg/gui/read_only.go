package gui

import (
	"fmt"
	"slices"

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

		view := binding.ViewName
		// A global key names no view: it acts on whichever has focus.
		if view == "" {
			view = gui.actionViewName(v)
		}

		if remote := gui.actionRemote(view); gui.isReadOnly(remote) {
			return gui.createErrorPanel(fmt.Sprintf(gui.Tr.ReadOnlyRemote, remote))
		}

		return handler(g, v)
	}
}

// actionViewName is the view a key acts on when its binding names none: the
// focused view where it is a side panel, and otherwise the last side panel to
// have had focus.
func (gui *Gui) actionViewName(v *gocui.View) string {
	if v != nil && slices.Contains(gui.sideViewNames(), v.Name()) {
		return v.Name()
	}

	return gui.currentSideViewName()
}

// actionRemote is the remote a key on view acts on: the selected stack's on
// Stacks, Services and Backups, and on Snapshots the remote of the instances it
// follows - either can be another remote's - and otherwise the session's.
// Listing every instance's, Snapshots lists the session's alone.
func (gui *Gui) actionRemote(view string) string {
	switch view {
	case "stacks", "services", "backups":
		if stack := gui.selectedStack.Load(); stack != nil && stack.Remote != "" {
			return stack.Remote
		}
	case "snapshots":
		if instances := gui.State.SnapshotsInstances; !gui.State.SnapshotsShowAll && len(instances) > 0 && instances[0].Remote != "" {
			return instances[0].Remote
		}
	}

	return gui.IncusCommand.RemoteName()
}

func (gui *Gui) isReadOnly(remote string) bool {
	return gui.Config.ReadOnly || gui.Config.UserConfig.Remotes[remote].ReadOnly
}
