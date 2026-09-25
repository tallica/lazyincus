// Copyright 2014 The gocui Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gui

import (
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
)

func (gui *Gui) wrappedConfirmationFunction(function func(*gocui.Gui, *gocui.View) error) func(*gocui.Gui, *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if err := gui.closeConfirmationPrompt(); err != nil {
			return err
		}

		if function != nil {
			if err := function(g, v); err != nil {
				return err
			}
		}

		return nil
	}
}

// dismissPrompt takes down a prompt and everything that came with it,
// without touching focus: switchFocus calls it once focus has landed
// somewhere else, so a popup can't be left on screen with no way back into
// it. closeConfirmationPrompt calls it on the way out too, and running
// twice over is harmless.
func (gui *Gui) dismissPrompt() {
	gui.Views.SnapshotOptions.Visible = false
	gui.g.DeleteViewKeybindings("snapshotOptions")

	gui.Views.Confirmation.Visible = false
	gui.Views.Confirmation.Subtitle = ""
	gui.g.DeleteViewKeybindings("confirmation")
}

func (gui *Gui) closeConfirmationPrompt() error {
	if err := gui.returnFocus(); err != nil {
		return err
	}
	gui.g.DeleteViewKeybindings("confirmation")
	gui.Views.Confirmation.Visible = false
	return nil
}

// popupColumns is where a popup sits across the screen: the middle half.
func popupColumns(screenWidth int) (int, int) {
	return screenWidth/2 - screenWidth/4, screenWidth/2 + screenWidth/4
}

// popupRows centres rows of content on the screen. A popup taller than the
// terminal ran off both ends of it rather than scrolling: focusPoint only
// moves a view's origin when the content doesn't fit the view. The clamp is
// what makes a long menu scroll.
func popupRows(screenHeight int, rows int) (int, int) {
	rows = max(1, min(rows, screenHeight-4))

	return screenHeight/2 - rows/2 - rows%2 - 1, screenHeight/2 + rows/2
}

// prepareConfirmationPanel opens the confirmation popup around prompt, sized
// to it from the first frame.
func (gui *Gui) prepareConfirmationPanel(title, prompt string) error {
	confirmationView := gui.Views.Confirmation
	confirmationView.SetOrigin(0, 0)
	confirmationView.SetCursor(0, 0)
	_ = gui.setViewContent(confirmationView, prompt)

	if err := gui.resizePopupPanel(confirmationView); err != nil {
		return err
	}

	confirmationView.Title = title
	confirmationView.Visible = true
	gui.g.Update(func(g *gocui.Gui) error {
		return gui.switchFocus(confirmationView)
	})
	return nil
}

func (gui *Gui) onNewPopupPanel() {
	gui.Views.Menu.Visible = false
	gui.Views.Confirmation.Visible = false
	gui.Views.SnapshotOptions.Visible = false
}

// It is very important that within this function we never include the original prompt in any error messages.
// nolint:unparam
func (gui *Gui) createConfirmationPanel(title, prompt string, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	return gui.createPopupPanel(title, prompt, handleConfirm, handleClose)
}

func (gui *Gui) createPopupPanel(title, prompt string, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	gui.onNewPopupPanel()
	gui.g.Update(func(g *gocui.Gui) error {
		if gui.currentViewName() == "confirmation" {
			if err := gui.closeConfirmationPrompt(); err != nil {
				gui.Log.Error(err.Error())
			}
		}
		err := gui.prepareConfirmationPanel(title, prompt)
		if err != nil {
			return err
		}
		gui.Views.Confirmation.Editable = false
		return gui.setKeyBindings(g, handleConfirm, handleClose)
	})
	return nil
}

func (gui *Gui) setKeyBindings(g *gocui.Gui, handleConfirm, handleClose func(*gocui.Gui, *gocui.View) error) error {
	confirm := gui.wrappedConfirmationFunction(handleConfirm)
	closeIt := gui.wrappedConfirmationFunction(handleClose)

	bindings := []struct {
		key     interface{}
		handler func(*gocui.Gui, *gocui.View) error
	}{
		{gocui.KeyEnter, confirm},
		{'y', confirm},
		{gocui.KeyEsc, closeIt},
		{'n', closeIt},
	}

	for _, binding := range bindings {
		if err := g.SetKeybinding("confirmation", binding.key, gocui.ModNone, binding.handler); err != nil {
			return err
		}
	}

	return nil
}

func (gui *Gui) createErrorPanel(message string) error {
	colorFunction := color.New(color.FgRed).SprintFunc()
	coloredMessage := colorFunction(strings.TrimSpace(message))
	return gui.createConfirmationPanel(gui.Tr.ErrorTitle, coloredMessage, nil, nil)
}

func (gui *Gui) renderConfirmationOptions() error {
	if gui.connectionPopupShowing() {
		return gui.renderConnectionLostOptions()
	}

	optionsMap := map[string]string{
		"n/esc":   gui.Tr.No,
		"y/enter": gui.Tr.Yes,
	}
	return gui.renderOptionsMap(optionsMap)
}
