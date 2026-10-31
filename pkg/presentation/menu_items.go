package presentation

// GetMenuItemDisplayStrings is the item's columns, a checklist's tick first.
func GetMenuItemDisplayStrings(columns []string, checked, checklist bool) []string {
	if !checklist {
		return columns
	}

	box := "[ ]"
	if checked {
		box = "[x]"
	}

	return append([]string{box}, columns...)
}
