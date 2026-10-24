package presentation

import "github.com/tallica/lazyincus/pkg/gui/types"

// GetMenuItemDisplayStrings is the item's columns, after a box saying
// whether it's ticked in a checklist.
func GetMenuItemDisplayStrings(menuItem *types.MenuItem, checklist bool) []string {
	if !checklist {
		return menuItem.LabelColumns
	}

	box := "[ ]"
	if menuItem.Checked {
		box = "[x]"
	}

	return append([]string{box}, menuItem.LabelColumns...)
}
