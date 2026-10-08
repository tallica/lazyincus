package types

type MenuItem struct {
	Label string

	// alternative to Label. Allows specifying columns which will be auto-aligned
	LabelColumns []string

	OnPress func() error

	// Only applies when Label is used
	OpensMenu bool

	// FilterText is what filtering the menu matches; the columns when empty.
	FilterText string

	// HideUntilFiltered keeps the item out of the menu until something is
	// typed in its filter.
	HideUntilFiltered bool

	// OnTab is what tab does on the item in a filtered menu; nil for
	// nothing.
	OnTab func() error

	// Keys are what a menu's own keys do on the item, beyond enter.
	Keys map[rune]MenuKey
}

// MenuKey is a key a menu item answers to. Mutates is guardReadOnly's
// Mutates, decided per item: the menu's keys mean something else in every
// menu.
type MenuKey struct {
	Handler func() error
	Mutates bool
}
