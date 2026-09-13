package gui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

// connectionState is the last verdict the modal acted on - the live one is
// IncusCommand.IsConnected() - so a drop is handled once, not every poll.
type connectionState struct {
	lost bool
}

// syncConnection pops a modal when the daemon stops answering and takes it
// down once it answers again. There's nothing to re-establish - the client
// dials per request - so the modal only reports what the poll finds. Called
// from that poll, hence the Update: the views are the main loop's.
func (gui *Gui) syncConnection() {
	connected := gui.IncusCommand.IsConnected()

	gui.g.Update(func(*gocui.Gui) error {
		if connected {
			gui.State.Connection.lost = false

			// Closed whenever it's up rather than on the transition
			// alone: createPopupPanel only queues the panel, so a
			// recovery within the tick can otherwise strand it.
			return gui.closeConnectionLostPanel()
		}

		// Not raised over a popup the user opened - that would wipe
		// half-typed input and leave its keybindings live behind ours.
		// Left for the next tick instead.
		if gui.State.Connection.lost || gui.popupPanelFocused() {
			return nil
		}

		gui.State.Connection.lost = true

		return gui.createConnectionLostPanel()
	})
}

func (gui *Gui) createConnectionLostPanel() error {
	prompt := fmt.Sprintf(gui.Tr.ConnectionLost, gui.IncusCommand.RemoteName)

	return gui.createConfirmationPanel(gui.Tr.ConnectionLostTitle, prompt, nil, nil)
}

func (gui *Gui) closeConnectionLostPanel() error {
	if !gui.connectionPopupShowing() {
		return nil
	}

	return gui.closeConfirmationPrompt()
}

// connectionPopupShowing reports whether the popup on screen is ours. Read
// off the view rather than tracked: esc, a keypress that moves focus and
// another popup all take it down without telling us.
func (gui *Gui) connectionPopupShowing() bool {
	return gui.Views.Confirmation.Visible &&
		gui.Views.Confirmation.Title == gui.Tr.ConnectionLostTitle
}

// renderConnectionLostOptions: nothing to confirm, just a notice to put away.
func (gui *Gui) renderConnectionLostOptions() error {
	return gui.renderOptionsMap(map[string]string{"esc/enter": gui.Tr.Close})
}
