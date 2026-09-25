package gui

import (
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/assert"
)

func TestGetGocuiAttributeHex(t *testing.T) {
	assert.Equal(t, gocui.NewRGBColor(0x12, 0x34, 0xab), GetGocuiAttribute("#1234AB"))
	assert.Equal(t, gocui.NewRGBColor(0xff, 0x00, 0x88), GetGocuiAttribute("#f08"))
	assert.Equal(t, gocui.ColorRed, GetGocuiAttribute("red"))
	assert.Equal(t, gocui.ColorDefault, GetGocuiAttribute("#12345"))
	assert.Equal(t, gocui.ColorDefault, GetGocuiAttribute("#12345g"))
}
