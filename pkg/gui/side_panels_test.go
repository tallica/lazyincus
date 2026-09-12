package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
)

func TestEverySidePanelDefHasAPanel(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}
	gui.setPanels()

	seen := map[string]bool{}

	for _, def := range gui.sidePanelDefs() {
		assert.NotNil(t, def.panel(), "no panel wired up for %q", def.name)
		assert.NotNil(t, def.viewPtr, "no view pointer for %q", def.name)
		assert.NotEmpty(t, def.title, "no title for %q", def.name)
		assert.False(t, seen[def.name], "duplicate side panel name %q", def.name)
		seen[def.name] = true
	}

	assert.Len(t, gui.allSidePanels(), len(gui.sidePanelDefs()))
}

func TestFocusKeysStartAtOne(t *testing.T) {
	assert.Equal(t, '1', focusKey(0))
	assert.Equal(t, '2', focusKey(1))

	gui := &Gui{}
	assert.Equal(t, "[1]", gui.sidePanelTitlePrefix(0))
	assert.Equal(t, "[3]", gui.sidePanelTitlePrefix(2))
}

func TestFocusPanelDescription(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}

	assert.Equal(t, "focus instances panel", gui.focusPanelDescription("Instances"))
}
