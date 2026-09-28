package gui

import (
	"strings"

	"github.com/jesseduffield/gocui"
)

// openTextPrompt asks for one line of text in the confirmation popup, made
// editable the way the snapshot prompt's name field is. enter takes the
// prompt down and hands submit what was typed; esc drops it.
func (gui *Gui) openTextPrompt(title, hint string, submit func(string) error) error {
	gui.onNewPopupPanel()

	if err := gui.prepareConfirmationPanel(title, ""); err != nil {
		return err
	}

	view := gui.Views.Confirmation
	view.Editable = true
	view.ClearTextArea()
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
			if text == "" {
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
