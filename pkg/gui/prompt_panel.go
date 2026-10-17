package gui

import (
	"errors"
	"strings"

	"github.com/jesseduffield/gocui"
)

// openTextPrompt asks for one line of text in the confirmation popup, made
// editable the way the snapshot prompt's name field is. enter takes the
// prompt down and hands submit what was typed, initial to start from; esc
// drops it.
func (gui *Gui) openTextPrompt(title, hint, initial string, submit func(string) error) error {
	return gui.textPrompt(title, hint, initial, refuseEmpty, submit)
}

// openOptionalTextPrompt is openTextPrompt where enter on nothing typed
// submits that.
func (gui *Gui) openOptionalTextPrompt(title, hint string, submit func(string) error) error {
	return gui.textPrompt(title, hint, "", acceptEmpty, submit)
}

// errKeepPrompt is a check's answer that enter should leave the prompt up and
// do nothing.
var errKeepPrompt = errors.New("keep prompt")

// refuseEmpty is the check for a prompt that has to be answered.
func refuseEmpty(text string) error {
	if text == "" {
		return errKeepPrompt
	}

	return nil
}

// acceptEmpty is the check for a prompt where enter on nothing typed is an
// answer in itself.
func acceptEmpty(string) error { return nil }

// textPrompt is openTextPrompt with the check it puts to the line spelled
// out, for a prompt that starts from a seed: nil submits what was typed,
// errKeepPrompt leaves the prompt up as it is, any other error leaves it up
// saying why in place of the hint.
func (gui *Gui) textPrompt(title, hint, initial string, check func(string) error, submit func(string) error) error {
	gui.onNewPopupPanel()

	if err := gui.prepareConfirmationPanel(title, ""); err != nil {
		return err
	}

	view := gui.Views.Confirmation
	view.Editable = true
	view.ClearTextArea()
	view.TextArea.TypeString(initial)
	view.RenderTextArea()
	// A subtitle rather than a footer: gocui skips the footer of a view with
	// no lines, which is how this one starts.
	view.Subtitle = hint

	closePrompt := func() error {
		gui.dismissPrompt()

		return gui.closeConfirmationPrompt()
	}

	bindings := map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(*gocui.Gui, *gocui.View) error {
			text := strings.TrimSpace(view.Buffer())

			if err := check(text); err != nil {
				if !errors.Is(err, errKeepPrompt) {
					view.Subtitle = err.Error()
				}

				return nil
			}

			if err := closePrompt(); err != nil {
				return err
			}

			return submit(text)
		},
		gocui.KeyEsc: func(*gocui.Gui, *gocui.View) error { return closePrompt() },
	}

	for key, handler := range bindings {
		if err := gui.g.SetKeybinding("confirmation", key, gocui.ModNone, handler); err != nil {
			return err
		}
	}

	return nil
}
