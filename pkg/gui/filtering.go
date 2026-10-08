package gui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

func (gui *Gui) handleOpenFilter() error {
	panel, ok := gui.currentListPanel()
	if !ok {
		return nil
	}

	if panel.IsFilterDisabled() {
		return nil
	}

	gui.State.Filter.active = true
	gui.State.Filter.panel = panel

	return gui.switchFocus(gui.Views.Filter)
}

// handleOpenSearch searches the main panel, the matches highlighted as you
// type; once the prompt is left, n and N step through them and esc clears it.
func (gui *Gui) handleOpenSearch() error {
	if gui.State.Filter.active {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	gui.State.Filter.active = true
	gui.State.Filter.search = true
	// Or a ticker's next write would scroll away from the match.
	gui.Views.Main.Autoscroll = false
	gui.renderFilterPrompt()

	return gui.switchFocus(gui.Views.Filter)
}

func (gui *Gui) onSearchEscape() error {
	if !gui.State.Filter.search {
		return nil
	}

	return gui.clearFilter()
}

func (gui *Gui) onNewFilterNeedle(value string) error {
	gui.State.Filter.needle = value
	if gui.State.Filter.search {
		if value == "" {
			gui.Views.Main.ClearSearch()
			gui.renderFilterPrompt()
		} else {
			gui.Views.Main.Search(value, nil)
		}

		return nil
	}

	gui.ResetOrigin(gui.State.Filter.panel.GetView())
	return gui.State.Filter.panel.RerenderList()
}

func (gui *Gui) wrapEditor(f func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool) func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		matched := f(v, key, ch, mod)
		if matched {
			if err := gui.onNewFilterNeedle(v.TextArea.GetContent()); err != nil {
				gui.Log.Error().Err(err).Send()
			}
		}
		return matched
	}
}

func (gui *Gui) escapeFilterPrompt() error {
	if gui.paletteOpen() {
		return gui.handleMenuClose()
	}

	search := gui.State.Filter.search
	if err := gui.clearFilter(); err != nil {
		return err
	}

	if search {
		return gui.leaveSearchPrompt()
	}

	return gui.returnFocus()
}

// leaveSearchPrompt returns to the main panel, taking the prompt off the
// view stack: returning to a list does that by itself, but returning to the
// main panel doesn't, and its esc would return to the prompt.
func (gui *Gui) leaveSearchPrompt() error {
	if err := gui.switchFocus(gui.Views.Main); err != nil {
		return err
	}

	gui.removeViewFromStack(gui.Views.Filter)

	return nil
}

func (gui *Gui) clearFilter() error {
	if gui.State.Filter.search {
		gui.Views.Main.ClearSearch()
	}

	gui.State.Filter.needle = ""
	gui.State.Filter.active = false
	gui.State.Filter.search = false
	panel := gui.State.Filter.panel
	gui.State.Filter.panel = nil
	gui.Views.Filter.ClearTextArea()
	gui.renderFilterPrompt()

	if panel == nil {
		return nil
	}

	gui.ResetOrigin(panel.GetView())

	return panel.RerenderList()
}

// returns to the list view with the filter still applied
func (gui *Gui) commitFilter() error {
	if gui.paletteOpen() {
		return gui.handleMenuPress()
	}

	search := gui.State.Filter.search
	if gui.State.Filter.needle == "" {
		if err := gui.clearFilter(); err != nil {
			return err
		}
	}

	if search {
		return gui.leaveSearchPrompt()
	}

	return gui.returnFocus()
}

// filteredPrevLine and filteredNextLine move through the list being filtered
// without leaving the filter.
func (gui *Gui) filteredPrevLine() error {
	if gui.State.Filter.panel == nil {
		return nil
	}

	return gui.State.Filter.panel.HandlePrevLine()
}

func (gui *Gui) filteredNextLine() error {
	if gui.State.Filter.panel == nil {
		return nil
	}

	return gui.State.Filter.panel.HandleNextLine()
}

func (gui *Gui) filterPrompt() string {
	if gui.paletteOpen() {
		return fmt.Sprintf("%s: ", gui.Tr.CommandPalettePrompt)
	}

	if gui.State.Filter.search {
		return gui.searchPrompt()
	}

	return fmt.Sprintf("%s: ", gui.Tr.FilterPrompt)
}

func (gui *Gui) searchPrompt() string {
	if !gui.Views.Main.IsSearching() {
		return fmt.Sprintf("%s: ", gui.Tr.SearchPrompt)
	}

	index, total := gui.Views.Main.GetSearchStatus()
	if total == 0 {
		return fmt.Sprintf("%s (%s): ", gui.Tr.SearchPrompt, gui.Tr.NoMatches)
	}

	return fmt.Sprintf("%s ("+gui.Tr.MatchOf+"): ", gui.Tr.SearchPrompt, index+1, total)
}

func (gui *Gui) renderFilterPrompt() {
	_ = gui.setViewContent(gui.Views.FilterPrefix, gui.filterPrompt())
}

// FilterString returns the current filter needle for the given view, if the
// view is the one currently being filtered.
func (gui *Gui) FilterString(view *gocui.View) string {
	if gui.State.Filter.panel == nil {
		return ""
	}
	if gui.State.Filter.panel.GetView() != view {
		return ""
	}
	return gui.State.Filter.needle
}
