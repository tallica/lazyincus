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
}
