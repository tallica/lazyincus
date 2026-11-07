package tui

import (
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
)

// list is a side panel's items, with the fetch, fill and selection every panel shares.
type list[T comparable] struct {
	panel *panel
	seq   *refreshSeq
	items []T
	// span: the items come from more than one project, so rows name theirs.
	span bool

	get     func(*commands.IncusCommand, *commands.Listing) ([]T, error)
	less    func(a, b T) bool
	key     func(T) string
	project func(T) string
	cells   func(item T, span bool) []string
	// spans, if set, replaces spansProjects over every item.
	spans func(items []T) bool
	// version is what, besides key, makes main's content stale.
	version func(T) string
	// filled, if set, runs after each set: what follows the items.
	filled func()
}

func (l *list[T]) fetch(m *Model) fetch { return l.fetchIn(m, nil) }

// fetchIn reads the items with incus, the only thing its closure takes off the loop.
func (l *list[T]) fetchIn(m *Model, listing *commands.Listing) fetch {
	ticket := l.seq.issue()
	incus := m.incus

	return func() (func(), error) {
		items, err := l.get(incus, listing)
		if err != nil {
			return nil, err
		}

		return func() {
			if l.seq.admit(ticket) {
				l.set(m, items)
			}
		}, nil
	}
}

func (l *list[T]) set(m *Model, items []T) {
	m.refill(l.panel, l.keyOf, func() [][]string {
		sort.SliceStable(items, func(a, b int) bool { return l.less(items[a], items[b]) })
		l.items = items
		switch {
		case l.spans != nil:
			l.span = l.spans(items)
		case l.project != nil:
			l.span = spansProjects(items, l.project)
		}

		cells := make([][]string, len(items))
		for i, item := range items {
			cells[i] = l.cells(item, l.span)
		}

		return cells
	})

	if l.filled != nil {
		l.filled()
	}
}

func (l *list[T]) keyOf(item int) string { return l.key(l.items[item]) }

// selected is the selected row's item, the zero value when there's none.
func (l *list[T]) selected() T {
	var none T

	item := l.panel.item()
	if item < 0 || item >= len(l.items) {
		return none
	}

	return l.items[item]
}

// itemKey names what main shows: the selected item and its version.
func (l *list[T]) itemKey() string {
	var none T

	item := l.selected()
	if item == none {
		return ""
	}

	return l.key(item) + "-" + l.version(item)
}

func (l *list[T]) selectedProject() string {
	var none T

	if item := l.selected(); item != none {
		return l.project(item)
	}

	return ""
}
