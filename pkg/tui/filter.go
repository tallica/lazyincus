package tui

import (
	"fmt"
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	filterView       = "filter"
	filterPrefixView = "filterPrefix"
	// promptColour is the FgColor pkg/gui gives the filter views.
	promptColour = "\x1b[32m"
)

// filter is the footer prompt: a list's filter, or the main panel's search.
type filter struct {
	active bool
	search bool
	// panel is the list being filtered, "" while searching.
	panel string
	input textinput.Model
	// ox is the gocui view origin that keeps the cursor in sight.
	ox int
}

func newFilterInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.SetVirtualCursor(false)
	input.KeyMap = editorKeys()
	input.Focus()

	return input
}

// editorKeys are gocui's SimpleEditor's, as far as textinput has them;
// its yank (ctrl+y) and overwrite (insert) it hasn't.
func editorKeys() textinput.KeyMap {
	keys := textinput.DefaultKeyMap()
	keys.WordForward.SetKeys("alt+right", "alt+f")
	keys.WordBackward.SetKeys("alt+left", "alt+b")
	keys.DeleteWordBackward.SetKeys("alt+backspace", "ctrl+w")
	keys.DeleteWordForward.SetEnabled(false)
	keys.Paste.SetEnabled(false)
	keys.AcceptSuggestion.SetEnabled(false)
	keys.NextSuggestion.SetEnabled(false)
	keys.PrevSuggestion.SetEnabled(false)

	return keys
}

// isEditorKey is whether SimpleEditor takes msg: a key it doesn't falls
// through to the global bindings.
func isEditorKey(msg tea.KeyPressMsg) bool {
	if msg.Text != "" {
		return true
	}

	k := editorKeys()
	bindings := []key.Binding{
		k.CharacterForward, k.CharacterBackward, k.WordForward, k.WordBackward,
		k.DeleteWordBackward, k.DeleteAfterCursor, k.DeleteBeforeCursor,
		k.DeleteCharacterBackward, k.DeleteCharacterForward, k.LineStart, k.LineEnd,
		key.NewBinding(key.WithKeys("ctrl+y", "insert")),
	}

	return key.Matches(msg, bindings...)
}

// openFilter is pkg/gui's handleOpenFilter; what was typed before stays.
func (m *Model) openFilter(p *panel) {
	m.filter.active = true
	m.filter.panel = p.name
	m.focusView(filterView)
}

// openSearch is pkg/gui's handleOpenSearch.
func (m *Model) openSearch() {
	if m.filter.active {
		m.clearFilter()
	}

	m.filter.active = true
	m.filter.search = true
	m.main.autoscroll = false
	m.focusView(filterView)
}

func (m *Model) filterKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.commitFilter()
	case "esc":
		m.escapeFilterPrompt()
	case "up":
		m.moveFilteredSelection(-1)
	case "down":
		m.moveFilteredSelection(1)
	case "tab":
		m.cycleWindows(1)
	default:
		if !isEditorKey(msg) {
			return false, nil
		}

		var cmd tea.Cmd
		m.filter.input, cmd = m.filter.input.Update(msg)
		m.onNeedle(m.filter.input.Value())

		return true, cmd
	}

	return true, nil
}

// onNeedle is pkg/gui's onNewFilterNeedle, which runs on every key the
// editor takes, so even moving the cursor searches afresh.
func (m *Model) onNeedle(needle string) {
	if !m.filter.search {
		m.refilter(m.filter.panel)
		return
	}

	if needle == "" {
		m.main.clearSearch()
		return
	}

	m.main.find(needle)
}

// searchKey is gocui's own n, N and esc for a view being searched, ahead
// of any binding.
func (m *Model) searchKey(key string) bool {
	if m.currentView() != mainView || !m.main.searching() {
		return false
	}

	switch key {
	case "n":
		m.main.nextMatch(1)
	case "N":
		m.main.nextMatch(-1)
	case "esc":
		m.main.clearSearch()
		if m.filter.search {
			m.clearFilter()
		}
	default:
		return false
	}

	return true
}

func (m *Model) escapeFilterPrompt() {
	search := m.filter.search
	m.clearFilter()

	if search {
		m.leaveSearchPrompt()
		return
	}

	m.returnFocus()
}

func (m *Model) commitFilter() {
	search := m.filter.search
	if m.filter.input.Value() == "" {
		m.clearFilter()
	}

	if search {
		m.leaveSearchPrompt()
		return
	}

	m.returnFocus()
}

// leaveSearchPrompt is pkg/gui's: back to main, the prompt off the stack,
// or main's esc would return to it.
func (m *Model) leaveSearchPrompt() {
	m.focusView(mainView)
	m.focus = slices.DeleteFunc(m.focus, func(view string) bool { return view == filterView })
}

func (m *Model) clearFilter() {
	if m.filter.search {
		m.main.clearSearch()
	}

	m.filter.active = false
	m.filter.search = false
	m.filter.input.Reset()
	m.filter.ox = 0

	name := m.filter.panel
	m.filter.panel = ""
	m.refilter(name)
}

// refilter is pkg/gui's ResetOrigin and RerenderList on a filtered list.
func (m *Model) refilter(name string) {
	p := m.panelNamed(name)
	if p == nil {
		return
	}

	p.origin = 0
	m.filterList(p)
	p.selectRow(p.selected, m.listRows(p))
}

// moveFilteredSelection is pkg/gui's filteredPrevLine and filteredNextLine.
func (m *Model) moveFilteredSelection(step int) {
	if p := m.panelNamed(m.filter.panel); p != nil {
		m.moveSelection(p, step)
	}
}

// filterFollowsFocus is pkg/gui's switchFocus ending a filter once its
// list is off the stack, a search once main is.
func (m *Model) filterFollowsFocus() {
	switch {
	case m.filter.panel != "" && !slices.Contains(m.focus, m.filter.panel):
		m.clearFilter()
	case m.filter.search && !slices.Contains(m.focus, mainView):
		m.clearFilter()
	}
}

func (m *Model) filterPrompt() string {
	if !m.filter.search {
		return m.tr.FilterPrompt + ": "
	}

	if !m.main.searching() {
		return m.tr.SearchPrompt + ": "
	}

	index, total := m.main.matchStatus()
	if total == 0 {
		return fmt.Sprintf("%s (%s): ", m.tr.SearchPrompt, m.tr.NoMatches)
	}

	return fmt.Sprintf("%s ("+m.tr.MatchOf+"): ", m.tr.SearchPrompt, index+1, total)
}

func (m *Model) filterCursorX() int {
	value := []rune(m.filter.input.Value())

	return ansi.StringWidth(string(value[:m.filter.input.Position()]))
}

// fitFilter is gocui's updatedCursorAndOrigin.
func (m *Model) fitFilter() {
	d, ok := m.dims()[filterView]
	if !ok {
		return
	}
	w, _ := size(d)

	cursor, usable := m.filterCursorX(), w-1
	switch {
	case cursor > m.filter.ox+usable:
		m.filter.ox = cursor - usable
	case cursor < m.filter.ox:
		m.filter.ox = cursor
	}
}

func (m *Model) filterLine(width int) string {
	value := m.filter.input.Value()

	return tint(ansi.Cut(value, m.filter.ox, m.filter.ox+width), promptColour)
}

func (m *Model) filterList(p *panel) {
	needle := ""
	if m.filter.panel == p.name {
		needle = m.filter.input.Value()
	}

	p.filter(needle, m.cfg.UserConfig.Ignore)
}
