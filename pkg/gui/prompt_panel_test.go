package gui

import (
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/assert"
)

// A text prompt's footer offers no y/n: those are letters to type.
func TestTextPromptFooter(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, nil)
	s.ready(t)

	s.do(t, func() error {
		return s.gui.openTextPrompt("Name", "", "", func(string) error { return nil })
	})

	screen := s.settle(t, "enter: submit")
	assert.NotContains(t, screen, "y/enter")

	s.press(t, 'y')
	s.press(t, 'n')
	s.settle(t, "│yn")
}

// A message's footer offers no yes or no: it isn't asking anything.
func TestMessagePanelFooter(t *testing.T) {
	s := startScreenWith(t, 140, 40, nil, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.createErrorPanel("it broke") })
	screen := s.settle(t, "esc/enter: close")
	assert.NotContains(t, screen, "y/enter")

	s.do(t, func() error {
		return s.gui.createConfirmationPanel("Confirm", "sure?", func(*gocui.Gui, *gocui.View) error { return nil }, nil)
	})
	s.settle(t, "y/enter: yes")
}
