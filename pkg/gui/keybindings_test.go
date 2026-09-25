package gui

import (
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/assert"
)

func TestBindingGetKey(t *testing.T) {
	cases := map[any]string{
		'x':                 "x",
		' ':                 "space",
		gocui.KeyEnter:      "enter",
		gocui.KeyBacktab:    "shift+tab",
		gocui.KeyArrowRight: "►",
		gocui.KeyPgdn:       "PgDn",
		gocui.KeyCtrlC:      "",
	}

	for key, label := range cases {
		assert.Equal(t, label, (&Binding{Key: key}).GetKey(), "%v", key)
	}

	assert.Empty(t, (&Binding{}).GetKey())
}
