package gui

import (
	"strconv"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/utils"
)

var gocuiColorMap = map[string]gocui.Attribute{
	"default":   gocui.ColorDefault,
	"black":     gocui.ColorBlack,
	"red":       gocui.ColorRed,
	"green":     gocui.ColorGreen,
	"yellow":    gocui.ColorYellow,
	"blue":      gocui.ColorBlue,
	"magenta":   gocui.ColorMagenta,
	"cyan":      gocui.ColorCyan,
	"white":     gocui.ColorWhite,
	"bold":      gocui.AttrBold,
	"reverse":   gocui.AttrReverse,
	"underline": gocui.AttrUnderline,
}

// GetGocuiAttribute gets the gocui color attribute from the string
func GetGocuiAttribute(key string) gocui.Attribute {
	if utils.IsValidHexValue(key) {
		return hexColor(key)
	}

	value, present := gocuiColorMap[key]
	if present {
		return value
	}
	return gocui.ColorDefault
}

// GetGocuiStyle bitwise OR's a list of attributes obtained via the given keys
func GetGocuiStyle(keys []string) gocui.Attribute {
	var attribute gocui.Attribute
	for _, key := range keys {
		attribute |= GetGocuiAttribute(key)
	}
	return attribute
}

// hexColor reads a colour IsValidHexValue has accepted: #rgb or #rrggbb.
func hexColor(hex string) gocui.Attribute {
	digits := hex[1:]
	if len(digits) == 3 {
		digits = strings.Repeat(digits[0:1], 2) + strings.Repeat(digits[1:2], 2) + strings.Repeat(digits[2:3], 2)
	}

	value, _ := strconv.ParseUint(digits, 16, 32)

	return gocui.NewRGBColor(int32(value>>16&0xff), int32(value>>8&0xff), int32(value&0xff))
}
