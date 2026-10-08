package gui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

// handleMenuKey runs the selected item's key, refused on a read-only remote
// when the key changes something there. A menu whose item doesn't answer
// to the key ignores it.
func (gui *Gui) handleMenuKey(key rune) func(*gocui.Gui, *gocui.View) error {
	return func(*gocui.Gui, *gocui.View) error {
		item, err := gui.Panels.Menu.GetSelectedItem()
		if err != nil {
			return nil
		}

		menuKey, ok := item.Keys[key]
		if !ok {
			return nil
		}

		if remote := gui.IncusCommand.RemoteName(); menuKey.Mutates && gui.isReadOnly(remote) {
			if gui.Config.ReadOnly {
				return gui.createErrorPanel(gui.Tr.ReadOnlySession)
			}

			return gui.createErrorPanel(fmt.Sprintf(gui.Tr.ReadOnlyRemote, remote))
		}

		return menuKey.Handler()
	}
}

// fetchWarnings lists the remote's warnings.
func (gui *Gui) fetchWarnings() (func() error, error) {
	ticket := gui.refreshes.warnings.issue()

	warnings, err := gui.IncusCommand.GetWarnings()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.warnings.admit(ticket) {
			return nil
		}

		return gui.showWarnings(warnings)
	}, nil
}

func (gui *Gui) refreshWarningsQuiet() error {
	if err := gui.refresh(nil, gui.fetchWarnings); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// showWarnings keeps the warnings, counts the new ones for the footer and
// redraws the popup if it's open on them. Main loop only.
func (gui *Gui) showWarnings(warnings []*commands.Warning) error {
	selected := gui.daemonSelected(daemonWarnings)

	sortWarnings(warnings)
	gui.State.Warnings = warnings

	newCount := 0
	for _, warning := range warnings {
		if warning.IsNew() {
			newCount++
		}
	}

	gui.newWarnings.Store(int32(newCount))

	if err := gui.renderString(gui.g, "information", gui.getInformationContent()); err != nil {
		return err
	}

	return gui.redrawDaemon(daemonWarnings, selected)
}

// sortWarnings puts new ones first, then the most severe, then the most
// recently seen.
func sortWarnings(warnings []*commands.Warning) {
	slices.SortStableFunc(warnings, func(a, b *commands.Warning) int {
		if a.IsNew() != b.IsNew() {
			if a.IsNew() {
				return -1
			}

			return 1
		}

		if bySeverity := cmp.Compare(presentation.WarningSeverityRank(b), presentation.WarningSeverityRank(a)); bySeverity != 0 {
			return bySeverity
		}

		return b.Warning.LastSeenAt.Compare(a.Warning.LastSeenAt)
	})
}

// warningItems are the popup's rows, and each one's warning's key.
func (gui *Gui) warningItems() ([]*types.MenuItem, []string) {
	items := make([]*types.MenuItem, 0, len(gui.State.Warnings))
	keys := make([]string, 0, len(gui.State.Warnings))

	for _, warning := range gui.State.Warnings {
		keys = append(keys, warning.Key())
		items = append(items, &types.MenuItem{
			LabelColumns: presentation.GetWarningDisplayStrings(warning),
			FilterText:   warning.Warning.Type + " " + warning.Warning.LastMessage,
			OnPress:      func() error { return gui.showWarning(warning) },
			Keys: map[rune]types.MenuKey{
				'a': {Handler: func() error { return gui.acknowledgeWarning(warning) }, Mutates: true},
				'd': {Handler: func() error { return gui.deleteWarning(warning) }, Mutates: true},
			},
		})
	}

	if len(items) == 0 {
		items = append(items, &types.MenuItem{LabelColumns: []string{gui.Tr.NoWarnings}})
	}

	return items, keys
}

// showWarning is enter on a warning: all of it, which the row has no room
// for. Closing it goes back to the list.
func (gui *Gui) showWarning(warning *commands.Warning) error {
	back := func(*gocui.Gui, *gocui.View) error { return gui.openDaemon(daemonWarnings, warning.Key()) }

	return gui.createConfirmationPanel(gui.Tr.WarningTitle, gui.warningDetails(warning), back, back)
}

func (gui *Gui) warningDetails(warning *commands.Warning) string {
	w := warning.Warning
	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", identityPadding) + value + "\n"
	}

	project := w.Project
	if project == "" {
		project = gui.Tr.WarningServerWide
	}

	details := line("Type", w.Type) +
		line("Message", w.LastMessage) +
		line("Severity", w.Severity) +
		line("Status", w.Status) +
		line("Project", project) +
		line("Entity", warning.Entity()) +
		line("Count", fmt.Sprint(w.Count)) +
		line("First seen", w.FirstSeenAt.Local().Format(presentation.SecondsFormat)) +
		line("Last seen", w.LastSeenAt.Local().Format(presentation.SecondsFormat))

	if w.Location != "" && w.Location != "none" {
		details += line("Member", w.Location)
	}

	return strings.TrimRight(details, "\n")
}

// acknowledgeWarning is `a`; the popup stays open, redrawn.
func (gui *Gui) acknowledgeWarning(warning *commands.Warning) error {
	return gui.WithWaitingStatus(gui.Tr.AcknowledgingStatus, func() error {
		if err := gui.IncusCommand.AcknowledgeWarning(warning); err != nil {
			return gui.createErrorPanel(err.Error())
		}

		return gui.refresh(nil, gui.fetchWarnings)
	})
}

// deleteWarning is `d`, asked first, then back to the list either way.
// The prompt shows all of the warning: two of a type in one project can
// differ only in their count and when they were first seen.
func (gui *Gui) deleteWarning(warning *commands.Warning) error {
	prompt := gui.warningDetails(warning) + "\n\n" + gui.Tr.ConfirmDeleteWarning

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := gui.IncusCommand.DeleteWarning(warning); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refresh(func() error { return gui.openDaemon(daemonWarnings, "") }, gui.fetchWarnings)
		})
	}, func(*gocui.Gui, *gocui.View) error { return gui.openDaemon(daemonWarnings, warning.Key()) })
}

// warningsStatusContent is the footer's count of new warnings.
func (gui *Gui) warningsStatusContent() string {
	count := gui.newWarnings.Load()
	if count == 0 {
		return ""
	}

	text := fmt.Sprintf(gui.Tr.WarningsCount, count)
	if count == 1 {
		text = gui.Tr.WarningCount
	}

	return utils.ColoredString(text, color.FgYellow) + "  "
}
