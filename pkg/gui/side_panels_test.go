package gui

import (
	"testing"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		if def.window != "" {
			assert.NotEmpty(t, def.shortTitle, "no short tab name for %q", def.name)
		}
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

func TestStacksAndServicesHiddenWithoutIncusCompose(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}

	names := func() []string {
		return lo.Map(gui.visibleSidePanelDefs(), func(def sidePanelDef, _ int) string { return def.name })
	}

	// No incus-compose: neither panel is there, and instances is `1` rather
	// than leaving a gap at it.
	assert.NotContains(t, names(), "stacks")
	assert.NotContains(t, names(), "services")
	assert.Equal(t, "instances", names()[0])

	gui.State.ComposeAvailable = true

	assert.Equal(t, []string{"stacks", "services", "instances"}, names()[:3])
}

func TestInstancesAreStandaloneOnceAStackHasTheirs(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}

	assert.Equal(t, gui.Tr.InstancesTitle, gui.instancesPanelTitle())

	gui.State.StackServices = map[string]map[string]bool{"shop": {"api": true}}

	assert.Equal(t, gui.Tr.StandaloneInstancesTitle, gui.instancesPanelTitle())
}

func TestResourcesShareAWindow(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}

	assert.Equal(t, []string{"instances", "snapshots", resourcesWindow}, gui.sideWindowNames())
	assert.Equal(t, resourcesWindow, gui.windowOfView("volumes"))
	assert.Equal(t, "instances", gui.windowOfView("instances"))
	assert.Equal(t, "main", gui.windowOfView("main"))

	assert.Equal(t, "images", gui.activeViewInWindow(resourcesWindow))
	assert.False(t, gui.isHiddenInWindow("images"))
	assert.True(t, gui.isHiddenInWindow("networks"))

	gui.noteActiveView("networks")
	gui.noteActiveView("instances")

	assert.Equal(t, "networks", gui.activeViewInWindow(resourcesWindow))
	assert.True(t, gui.isHiddenInWindow("images"))
	assert.False(t, gui.isHiddenInWindow("instances"))
}

func TestFocusPanelDescription(t *testing.T) {
	gui := &Gui{Tr: i18n.NewTranslationSet(commands.NewDummyLog(), "en")}

	assert.Equal(t, "focus instances panel", gui.focusPanelDescription("Instances"))
}

func TestSpansMultipleProjects(t *testing.T) {
	assert.False(t, spansMultipleProjects(nil))
	assert.False(t, spansMultipleProjects([]string{"default", "default"}))
	assert.False(t, spansMultipleProjects([]string{"", "default", ""}))
	assert.True(t, spansMultipleProjects([]string{"default", "demo"}))
	assert.True(t, spansMultipleProjects([]string{"demo", "", "default"}))
}

// The list the main panel shows keeps its selection, in the inactive
// style, while the main panel has the focus; another list's goes.
func TestTheShownListKeepsItsSelectionWhileTheMainPanelIsFocused(t *testing.T) {
	s := startScreen(t, 140, 40, nil)
	s.ready(t)

	s.do(t, func() error { return s.gui.switchFocus(s.gui.Views.Main) })

	highlight := func(view *gocui.View) [2]bool {
		return onLoop(t, s, func() [2]bool { return [2]bool{view.Highlight, view.HighlightInactive} })
	}
	require.Eventually(t, func() bool { return highlight(s.gui.Views.Instances) == [2]bool{true, true} }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, [2]bool{false, false}, highlight(s.gui.Views.Snapshots))
	assert.Equal(t, [2]bool{false, false}, highlight(s.gui.Views.Main))
}
