package panels

import (
	"context"
	"sort"
	"strings"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/sahilm/fuzzy"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

type ISideListPanel interface {
	Await(note string)
	StopAwaiting() bool
	SetMainTabIndex(int)
	SetMainTab(string) error
	HandleSelect() error
	GetView() *gocui.View
	Refocus()
	RerenderList() error
	FitToWidth()
	IsFilterDisabled() bool
	IsHidden() bool
	HandleNextLine() error
	HandlePrevLine() error
	HandleClick() error
	HandlePrevMainTab() error
	HandleNextMainTab() error
}

// list panel at the side of the screen that renders content to the main panel
type SideListPanel[T comparable] struct {
	ContextState *ContextState[T]

	ListPanel[T]

	// message to render in the main view if there are no items in the panel
	// and it has focus. Leave empty if you don't want to render anything
	NoItemsMessage string

	// EmptyNote is a line the list itself shows while it has no rows, for
	// an empty that's worth noticing without focusing the panel.
	EmptyNote func() string

	// awaiting stands in for the list, and in the main panel, until items
	// are next set: a read not answered yet, rather than nothing to list.
	awaiting string

	// a representation of the gui
	Gui IGui

	// this Filter is applied on top of additional default filters
	Filter func(T) bool
	Sort   func(a, b T) bool

	// a callback to invoke when the item is clicked
	OnClick func(T) error

	// a callback to invoke when a new item is selected (e.g. keyboard navigation)
	OnSelect func(T) error

	// SameItem reports whether two items are the same row. Only panels whose
	// items a refresh rebuilds need it: the cursor follows the selected item
	// by identity, and a replaced pointer otherwise leaves it holding an
	// index, which by then belongs to a different row.
	SameItem func(a, b T) bool

	// returns the cells that we render to the view in a table format. The cells will
	// be rendered with padding.
	GetTableCells func(T) []string

	// function to be called after re-rendering list. Can be nil
	OnRerender func() error

	// set this to true if you don't want to allow manual filtering via '/'
	DisableFilter bool

	// FuzzyFilter matches each word of the filter fuzzily, in any order,
	// best match first, and puts the cursor on it.
	FuzzyFilter bool

	// FilterText is what the fuzzy filter matches; the cells when nil or "".
	FilterText func(T) string

	// This can be nil if you want to always show the panel
	Hide func() bool

	// FlexColumns take the width the rest leave, giving it up in order when
	// there's too little; nil for none. A func, since a project column ahead
	// of one comes and goes.
	FlexColumns func() []utils.FlexColumn

	// the rows' cells, and the width they were last rendered at, so a
	// resize re-renders them without asking for the cells again
	rows      [][]string
	clipWidth int
}

var _ ISideListPanel = &SideListPanel[int]{}

type IGui interface {
	HandleClick(v *gocui.View, itemCount int, selectedLine *int, handleSelect func() error) error
	NewSimpleRenderStringTask(getContent func() string) tasks.TaskFunc
	FocusY(selectedLine int, itemCount int, view *gocui.View)
	ShouldRefresh(contextKey string) bool
	GetMainView() *gocui.View
	IsCurrentView(*gocui.View) bool
	FilterString(view *gocui.View) string
	IgnoreStrings() []string

	QueueTask(f func(ctx context.Context)) error
}

func (self *SideListPanel[T]) HandleClick() error {
	itemCount := self.List.Len()
	handleSelect := self.HandleSelect
	selectedLine := &self.SelectedIdx

	if err := self.Gui.HandleClick(self.View, itemCount, selectedLine, handleSelect); err != nil {
		return err
	}

	if self.OnClick != nil {
		selectedItem, err := self.GetSelectedItem()
		if err == nil {
			return self.OnClick(selectedItem)
		}
	}

	return nil
}

func (self *SideListPanel[T]) GetView() *gocui.View {
	return self.View
}

func (self *SideListPanel[T]) HandleSelect() error {
	item, err := self.GetSelectedItem()
	if err != nil {
		if err.Error() != self.NoItemsMessage {
			return err
		}

		message := self.NoItemsMessage
		if self.awaiting != "" {
			message = self.awaiting
		}

		if message != "" {
			// Queued, not just built: an unqueued task renders nothing, which
			// left the previous panel's content on screen when you focused an
			// empty one.
			task := self.Gui.NewSimpleRenderStringTask(func() string { return message })

			mainView := self.Gui.GetMainView()
			mainView.Tabs = nil
			mainView.TabIndex = 0

			return self.Gui.QueueTask(task)
		}

		return nil
	}

	self.Refocus()

	if self.OnSelect != nil {
		if err := self.OnSelect(item); err != nil {
			return err
		}
	}

	return self.renderContext(item)
}

func (self *SideListPanel[T]) renderContext(item T) error {
	if self.ContextState == nil {
		return nil
	}

	key := self.ContextState.GetCurrentContextKey(item)
	if !self.Gui.ShouldRefresh(key) {
		return nil
	}

	mainView := self.Gui.GetMainView()
	mainView.Tabs = self.ContextState.GetMainTabTitles()
	mainView.TabIndex = self.ContextState.mainTabIdx

	task := self.ContextState.GetCurrentMainTab().Render(item)

	return self.Gui.QueueTask(task)
}

func (self *SideListPanel[T]) GetSelectedItem() (T, error) {
	var zero T

	item, ok := self.List.TryGet(self.SelectedIdx)
	if !ok {
		// could probably have a better error here
		return zero, errors.New(self.NoItemsMessage)
	}

	return item, nil
}

func (self *SideListPanel[T]) HandleNextLine() error {
	self.SelectNextLine()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandlePrevLine() error {
	self.SelectPrevLine()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandleNextMainTab() error {
	if self.ContextState == nil {
		return nil
	}

	self.ContextState.HandleNextMainTab()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) HandlePrevMainTab() error {
	if self.ContextState == nil {
		return nil
	}

	self.ContextState.HandlePrevMainTab()

	return self.HandleSelect()
}

func (self *SideListPanel[T]) Refocus() {
	self.Gui.FocusY(self.SelectedIdx, self.List.Len(), self.View)
}

// Await empties the list, note standing in for it until items are next set.
// Typed SetItems isn't reachable through ISideListPanel, so this is how
// generic code drops a panel's contents.
func (self *SideListPanel[T]) Await(note string) {
	self.SetItems(nil)
	self.awaiting = note
}

// StopAwaiting drops the note Await left, if no items have come since, and
// says whether it did.
func (self *SideListPanel[T]) StopAwaiting() bool {
	awaiting := self.awaiting != ""
	self.awaiting = ""

	return awaiting
}

func (self *SideListPanel[T]) SetItems(items []T) {
	self.awaiting = ""

	// Read the selection before the list is replaced, not after: by then the
	// index points into the new items and would anchor on the wrong one.
	selected, hadSelection := self.List.TryGet(self.SelectedIdx)

	self.List.SetItems(items)
	self.filterAndSort(selected, hadSelection)
}

func (self *SideListPanel[T]) FilterAndSort() {
	selected, hadSelection := self.List.TryGet(self.SelectedIdx)

	self.filterAndSort(selected, hadSelection)
}

func (self *SideListPanel[T]) filterAndSort(selected T, hadSelection bool) {
	filterString := self.Gui.FilterString(self.View)
	fuzzyFilter := self.FuzzyFilter && filterString != ""

	self.List.Filter(func(item T, index int) bool {
		if self.Filter != nil && !self.Filter(item) {
			return false
		}

		if lo.SomeBy(self.Gui.IgnoreStrings(), func(ignore string) bool {
			return lo.SomeBy(self.GetTableCells(item), func(searchString string) bool {
				return strings.Contains(searchString, ignore)
			})
		}) {
			return false
		}

		if filterString != "" && !fuzzyFilter {
			return lo.SomeBy(self.GetTableCells(item), func(searchString string) bool {
				return strings.Contains(searchString, filterString)
			})
		}

		return true
	})

	if fuzzyFilter {
		self.fuzzyFilter(filterString)
	} else {
		self.List.Sort(self.Sort)
	}

	self.clampSelectedLineIdx()

	// Follow the selected item to wherever it sorted to. The list re-sorts on
	// every background refresh, so holding the cursor at a fixed index would
	// hand the selection to a different item the moment one changes state.
	if hadSelection && !fuzzyFilter {
		if index := self.selectedIndex(selected); index >= 0 {
			self.SelectedIdx = index
		}
	}
}

// fuzzyFilter keeps the items matching every word of needle, best first.
func (self *SideListPanel[T]) fuzzyFilter(needle string) {
	items := self.List.GetItems()
	texts := lo.Map(items, func(item T, _ int) string {
		if self.FilterText != nil {
			if text := self.FilterText(item); text != "" {
				return text
			}
		}

		return utils.Decolorise(strings.Join(self.GetTableCells(item), " "))
	})

	words := strings.Fields(needle)
	scores := make([]int, len(items))
	matched := make([]int, len(items))
	literal := make([]int, len(items))
	for _, word := range words {
		for _, match := range fuzzy.FindNoSort(word, texts) {
			// Words as typed rank by how they appear, then in list order: their
			// fuzzy scores differ by what else the text holds.
			tier := wordTier(texts[match.Index], word)
			matched[match.Index]++
			if tier > 0 {
				scores[match.Index] += 1000 * tier
				literal[match.Index]++
			} else {
				scores[match.Index] += match.Score
			}
		}
	}

	// Scattered matches are only worth showing when nothing has the words
	// as typed.
	counts := matched
	if lo.Contains(literal, len(words)) {
		counts = literal
	}
	indices := lo.Filter(lo.Range(len(items)), func(index int, _ int) bool { return counts[index] == len(words) })
	sort.SliceStable(indices, func(i, j int) bool { return scores[indices[i]] > scores[indices[j]] })

	rank := map[T]int{}
	for position, index := range indices {
		rank[items[index]] = position
	}

	self.List.Filter(func(item T, _ int) bool {
		_, ok := rank[item]
		return ok
	})
	self.List.Sort(func(a, b T) bool { return rank[a] < rank[b] })
	self.SelectedIdx = 0
}

// wordTier ranks how word appears in text: 3 as a whole word, 2 starting
// one, 1 inside one, 0 only scattered. The fuzzy score alone can't: its
// greedy match takes the "st" of "instances" before the "stop" after it.
func wordTier(text, word string) int {
	text, word = strings.ToLower(text), strings.ToLower(word)
	tier := 0
	for field := range strings.FieldsSeq(text) {
		switch {
		case field == word:
			return 3
		case strings.HasPrefix(field, word):
			tier = 2
		case tier == 0 && strings.Contains(field, word):
			tier = 1
		}
	}

	return tier
}

// Select puts the cursor on item, and says whether the list shows it. A
// refresh may have rebuilt it since, so it's matched as selectedIndex
// matches.
func (self *SideListPanel[T]) Select(item T) bool {
	index := self.selectedIndex(item)
	if index < 0 {
		return false
	}

	self.SetSelectedLineIdx(index)

	return true
}

// IsSelected says whether the cursor is on item, matched as selectedIndex
// matches.
func (self *SideListPanel[T]) IsSelected(item T) bool {
	selected, err := self.GetSelectedItem()
	if err != nil {
		return false
	}

	return selected == item || (self.SameItem != nil && self.SameItem(item, selected))
}

// selectedIndex is where the previously selected item sorted to, by value
// and then - for panels that say so - by identity.
func (self *SideListPanel[T]) selectedIndex(selected T) int {
	if index := self.List.GetIndex(selected); index >= 0 {
		return index
	}

	if self.SameItem == nil {
		return -1
	}

	return self.List.GetIndexBy(func(item T) bool { return self.SameItem(selected, item) })
}

// RerenderList re-filters and redraws the list. Main loop only, like
// everything else that touches a view.
func (self *SideListPanel[T]) RerenderList() error {
	self.FilterAndSort()

	self.rows = lo.Map(self.List.GetItems(), func(item T, index int) []string {
		return self.GetTableCells(item)
	})

	if err := self.writeRows(); err != nil {
		return err
	}

	if self.OnRerender != nil {
		if err := self.OnRerender(); err != nil {
			return err
		}
	}

	if self.showsMain() {
		return self.HandleSelect()
	}

	return nil
}

// showsMain is whether the main panel is this panel's: the panel focused,
// or the main panel entered from it, a tab being read - which is when a
// change the new items bring has to reach it.
func (self *SideListPanel[T]) showsMain() bool {
	if self.Gui.IsCurrentView(self.View) {
		return true
	}

	mainView := self.Gui.GetMainView()

	return mainView != nil && self.Gui.IsCurrentView(mainView) && mainView.ParentView == self.View
}

// FitToWidth re-renders the rows if the panel has changed width since they
// were written. The layout calls it, so a resize is drawn at the new width
// in the same frame rather than at the panel's next refresh.
func (self *SideListPanel[T]) FitToWidth() {
	if self.View.InnerWidth() != self.clipWidth {
		// The cells rendered once already, in RerenderList.
		_ = self.writeRows()
	}
}

// writeRows renders the table to the view's width, each row cut to it and
// the cut marked: gocui stops a long row at the edge without a sign. The
// mark goes in the row because drawFrame puts the scrollbar in the border.
func (self *SideListPanel[T]) writeRows() error {
	self.clipWidth = self.View.InnerWidth()

	var flex []utils.FlexColumn
	if self.FlexColumns != nil {
		flex = self.FlexColumns()
	}

	table, err := utils.RenderTableToWidth(self.rows, self.clipWidth, flex)
	if err != nil {
		return err
	}

	if len(self.rows) == 0 {
		switch {
		case self.awaiting != "":
			table = self.awaiting
		case self.EmptyNote != nil:
			table = self.EmptyNote()
		}
	}

	rows := strings.Split(table, "\n")
	for index, row := range rows {
		rows[index] = utils.Truncate(row, self.clipWidth)
	}

	self.View.SetContent(strings.Join(rows, "\n"))

	return nil
}

func (self *SideListPanel[T]) SetMainTabIndex(index int) {
	if self.ContextState == nil {
		return
	}

	self.ContextState.SetMainTabIndex(index)
}

func (self *SideListPanel[T]) SetMainTab(key string) error {
	if self.ContextState == nil || !self.ContextState.SetMainTab(key) {
		return nil
	}

	return self.HandleSelect()
}

func (self *SideListPanel[T]) IsFilterDisabled() bool {
	return self.DisableFilter
}

func (self *SideListPanel[T]) IsHidden() bool {
	if self.Hide == nil {
		return false
	}

	return self.Hide()
}
