package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getSnapshotsPanel() *panels.SideListPanel[*commands.Snapshot] {
	return &panels.SideListPanel[*commands.Snapshot]{
		ContextState: &panels.ContextState[*commands.Snapshot]{
			GetMainTabs: func() []panels.MainTab[*commands.Snapshot] {
				return []panels.MainTab[*commands.Snapshot]{
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderSnapshotConfig,
					},
				}
			},
			GetItemContextCacheKey: func(snapshot *commands.Snapshot) string {
				return "snapshots-" + snapshot.Key()
			},
		},
		ListPanel: panels.ListPanel[*commands.Snapshot]{
			List: panels.NewFilteredList[*commands.Snapshot](),
			View: gui.Views.Snapshots,
		},
		NoItemsMessage: gui.Tr.NoSnapshots,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Snapshot, b *commands.Snapshot) bool {
			// Newest first: a rollback almost always means the last one.
			return a.Snapshot.CreatedAt.After(b.Snapshot.CreatedAt)
		},
		GetTableCells: presentation.GetSnapshotDisplayStrings,
	}
}

func (gui *Gui) renderSnapshotConfig(snapshot *commands.Snapshot) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.snapshotConfigStr(snapshot) })
}

func (gui *Gui) snapshotConfigStr(snapshot *commands.Snapshot) string {
	padding := 12
	output := ""
	output += utils.WithPadding("Instance: ", padding) + snapshot.InstanceName + "\n"
	output += utils.WithPadding("Name: ", padding) + snapshot.Name + "\n"
	output += utils.WithPadding("Taken at: ", padding) + snapshot.Snapshot.CreatedAt.String() + "\n"
	output += utils.WithPadding("Stateful: ", padding) + fmt.Sprint(snapshot.Snapshot.Stateful) + "\n"

	data, err := utils.MarshalIntoYaml(snapshot.Snapshot)
	if err != nil {
		return fmt.Sprintf("Error marshalling snapshot details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

// refreshSnapshots reloads the panel for whichever instance is selected.
// Unlike the other panels this one follows another panel's selection, so it
// runs on selection changes as well as on its own poll.
func (gui *Gui) refreshSnapshots() error {
	if gui.Views.Snapshots == nil {
		return nil
	}

	instance, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		gui.Panels.Snapshots.ClearItems()
		gui.setSnapshotsTitle("")

		return gui.Panels.Snapshots.RerenderList()
	}

	gui.setSnapshotsTitle(instance.Name)

	snapshots, err := instance.Snapshots()
	if err != nil {
		return err
	}

	gui.Panels.Snapshots.SetItems(snapshots)

	return gui.Panels.Snapshots.RerenderList()
}

// setSnapshotsTitle names the instance the panel is showing, since the list
// alone gives no clue which instance these snapshots belong to.
func (gui *Gui) setSnapshotsTitle(instanceName string) {
	title := gui.Tr.SnapshotsTitle
	if instanceName != "" {
		title += " (" + instanceName + ")"
	}

	gui.Views.Snapshots.Title = title
}

func (gui *Gui) refreshSnapshotsQuiet() error {
	if err := gui.refreshSnapshots(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// snapshotPrompt is the state behind the new-snapshot popup: a name field,
// and an options box holding the rest of what `incus snapshot create` takes.
// The options are fields rather than actions - a row shows a value you
// change in place, and enter always means create, wherever the focus is.
type snapshotPrompt struct {
	instance *commands.Instance
	expiry   int
	stateful bool
	field    int
}

// snapshotExpiries is the cycle the expiry field runs through.
var snapshotExpiries = []struct {
	label string
	value time.Duration
}{
	{"never", 0},
	{"1 day", 24 * time.Hour},
	{"7 days", 7 * 24 * time.Hour},
	{"30 days", 30 * 24 * time.Hour},
}

const (
	snapshotFieldExpiry = iota
	snapshotFieldStateful
	snapshotFieldCount
)

func (p *snapshotPrompt) moveField(delta int) {
	p.field = (p.field + delta + snapshotFieldCount) % snapshotFieldCount
}

// changeField cycles the focused field's value. Both fields wrap, so left
// and right work on either of them without dead ends.
func (p *snapshotPrompt) changeField(delta int) {
	switch p.field {
	case snapshotFieldExpiry:
		p.expiry = (p.expiry + delta + len(snapshotExpiries)) % len(snapshotExpiries)
	case snapshotFieldStateful:
		p.stateful = !p.stateful
	}
}

func (p *snapshotPrompt) options() commands.SnapshotOptions {
	return commands.SnapshotOptions{
		Stateful:  p.stateful,
		ExpiresIn: snapshotExpiries[p.expiry].value,
	}
}

func (gui *Gui) handleSnapshotCreate(g *gocui.Gui, v *gocui.View) error {
	instance, err := gui.Panels.Instances.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := &snapshotPrompt{instance: instance}

	gui.onNewPopupPanel()

	if err := gui.prepareConfirmationPanel(fmt.Sprintf(gui.Tr.SnapshotNamePrompt, instance.Name), ""); err != nil {
		return err
	}

	gui.Views.Confirmation.Editable = true
	gui.Views.Confirmation.ClearTextArea()
	// Subtitle only: gocui skips a footer on a view with no lines, and this
	// one starts empty. The submit hint lives on the options box, which
	// always has content.
	gui.Views.Confirmation.Subtitle = gui.Tr.SnapshotSwitchFocusHint

	if err := gui.setSnapshotPromptKeyBindings(prompt); err != nil {
		return err
	}

	// Queued, not called: prepareConfirmationPanel switches focus through an
	// update of its own, and the options are positioned against the prompt's
	// final dimensions.
	gui.g.Update(func(*gocui.Gui) error {
		return gui.renderSnapshotOptions(prompt)
	})

	return nil
}

// setSnapshotPromptKeyBindings wires both halves of the popup. Bound on the
// views directly rather than through createPopupPanel, which assumes enter
// confirms and closes: here enter creates from either box, while the options
// need their own navigation.
func (gui *Gui) setSnapshotPromptKeyBindings(prompt *snapshotPrompt) error {
	create := func(g *gocui.Gui, v *gocui.View) error {
		return gui.submitSnapshotPrompt(prompt)
	}

	closePrompt := func(g *gocui.Gui, v *gocui.View) error {
		return gui.closeSnapshotPrompt()
	}

	moveField := func(delta int) func(*gocui.Gui, *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			prompt.moveField(delta)
			return gui.renderSnapshotOptions(prompt)
		}
	}

	changeField := func(delta int) func(*gocui.Gui, *gocui.View) error {
		return func(g *gocui.Gui, v *gocui.View) error {
			prompt.changeField(delta)
			return gui.renderSnapshotOptions(prompt)
		}
	}

	bindings := []struct {
		view    string
		key     gocui.Key
		handler func(*gocui.Gui, *gocui.View) error
	}{
		{"confirmation", gocui.KeyEnter, create},
		{"confirmation", gocui.KeyCtrlS, create},
		{"confirmation", gocui.KeyEsc, closePrompt},
		{"confirmation", gocui.KeyTab, func(g *gocui.Gui, v *gocui.View) error {
			return gui.focusSnapshotView(prompt, gui.Views.SnapshotOptions)
		}},

		{"snapshotOptions", gocui.KeyEnter, create},
		{"snapshotOptions", gocui.KeyCtrlS, create},
		{"snapshotOptions", gocui.KeyEsc, closePrompt},
		{"snapshotOptions", gocui.KeyArrowDown, moveField(1)},
		{"snapshotOptions", gocui.KeyArrowUp, moveField(-1)},
		{"snapshotOptions", gocui.KeyArrowRight, changeField(1)},
		{"snapshotOptions", gocui.KeyArrowLeft, changeField(-1)},
		{"snapshotOptions", gocui.KeySpace, changeField(1)},
		{"snapshotOptions", gocui.KeyTab, func(g *gocui.Gui, v *gocui.View) error {
			return gui.focusSnapshotView(prompt, gui.Views.Confirmation)
		}},
	}

	for _, binding := range bindings {
		if err := gui.g.SetKeybinding(binding.view, binding.key, gocui.ModNone, binding.handler); err != nil {
			return err
		}
	}

	// Letters only on the options: the name field is editable, so these
	// would be typed rather than navigating. `q` included because the view
	// isn't editable either, and the global quit binding would otherwise
	// close the app from inside a modal.
	for key, handler := range map[rune]func(*gocui.Gui, *gocui.View) error{
		'j': moveField(1),
		'k': moveField(-1),
		'l': changeField(1),
		'h': changeField(-1),
		'q': closePrompt,
	} {
		if err := gui.g.SetKeybinding("snapshotOptions", key, gocui.ModNone, handler); err != nil {
			return err
		}
	}

	return nil
}

func (gui *Gui) focusSnapshotView(prompt *snapshotPrompt, view *gocui.View) error {
	if err := gui.switchFocus(view); err != nil {
		return err
	}

	return gui.renderSnapshotOptions(prompt)
}

func (gui *Gui) submitSnapshotPrompt(prompt *snapshotPrompt) error {
	name := strings.TrimSpace(gui.Views.Confirmation.Buffer())
	if name == "" {
		// Nothing to create yet; put the cursor where the name goes.
		return gui.focusSnapshotView(prompt, gui.Views.Confirmation)
	}

	if err := gui.closeSnapshotPrompt(); err != nil {
		return err
	}

	return gui.createSnapshot(prompt.instance, name, prompt.options())
}

// renderSnapshotOptions draws the fields into their view, parked directly
// under the name field.
func (gui *Gui) renderSnapshotOptions(prompt *snapshotPrompt) error {
	rows := []struct {
		label string
		value string
	}{
		{gui.Tr.SnapshotExpiryField, snapshotExpiries[prompt.expiry].label},
		{gui.Tr.SnapshotStatefulField, gui.yesNo(prompt.stateful)},
	}

	lines := make([]string, len(rows))

	for index, row := range rows {
		// Brackets mark the value ← → would change. Non-selected rows keep
		// the same width so the values stay in a column.
		value := "  " + row.value + "  "
		if index == prompt.field {
			value = "‹ " + row.value + " ›"
		}

		lines[index] = " " + utils.WithPadding(row.label, 12) + value
	}

	view := gui.Views.SnapshotOptions
	view.Clear()
	fmt.Fprint(view, strings.Join(lines, "\n"))

	// The highlight is the selection: gocui draws it on the cursor line, so
	// the cursor is what moves rather than a marker in the text.
	if err := view.SetCursor(0, prompt.field); err != nil {
		return err
	}
	view.Title = gui.Tr.SnapshotOptionsTitle
	view.Subtitle = gui.Tr.SnapshotChangeHint
	view.Footer = gui.Tr.SnapshotSubmitHint
	view.Visible = true

	x0, _, x1, y1 := gui.Views.Confirmation.Dimensions()
	_, err := gui.g.SetView("snapshotOptions", x0, y1+1, x1, y1+2+len(lines), 0)

	return err
}

func (gui *Gui) yesNo(value bool) string {
	if value {
		return gui.Tr.Yes
	}

	return gui.Tr.No
}

func (gui *Gui) closeSnapshotPrompt() error {
	gui.dismissPrompt()

	return gui.closeConfirmationPrompt()
}

// renderSnapshotOptionsKeys fills the bottom line while the options have
// focus, the same way every other panel does.
func (gui *Gui) renderSnapshotOptionsKeys() error {
	return gui.renderOptionsMap(map[string]string{
		"esc":   gui.Tr.Close,
		"↑ ↓":   gui.Tr.Navigate,
		"← →":   gui.Tr.SnapshotChangeValue,
		"tab":   gui.Tr.SnapshotFocusName,
		"enter": gui.Tr.SnapshotCreate,
	})
}

func (gui *Gui) createSnapshot(instance *commands.Instance, name string, opts commands.SnapshotOptions) error {
	return gui.WithWaitingStatus(gui.Tr.SnapshottingStatus, func() error {
		if err := instance.CreateSnapshot(name, opts); err != nil {
			return gui.createErrorPanel(err.Error())
		}

		if err := gui.refreshSnapshots(); err != nil {
			return err
		}

		return gui.focusSnapshot(name)
	})
}

// focusSnapshot moves to the snapshots panel and puts the cursor on the
// named snapshot, so a snapshot taken from the instances panel lands you
// where you can see it.
func (gui *Gui) focusSnapshot(name string) error {
	index := lo.IndexOf(lo.Map(gui.Panels.Snapshots.List.GetItems(),
		func(snapshot *commands.Snapshot, _ int) string { return snapshot.Name }), name)
	if index < 0 {
		return nil
	}

	gui.Panels.Snapshots.SetSelectedLineIdx(index)

	// This runs on the waiting-status goroutine; focus belongs to the main
	// loop.
	gui.g.Update(func(*gocui.Gui) error {
		return gui.switchFocus(gui.Views.Snapshots)
	})

	return nil
}

func (gui *Gui) handleSnapshotRestore(g *gocui.Gui, v *gocui.View) error {
	snapshot, err := gui.Panels.Snapshots.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.RestoreSnapshot, snapshot.InstanceName, snapshot.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RestoringStatus, func() error {
			if err := snapshot.Restore(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshInstances()
		})
	}, nil)
}

func (gui *Gui) handleSnapshotDelete(g *gocui.Gui, v *gocui.View) error {
	snapshot, err := gui.Panels.Snapshots.GetSelectedItem()
	if err != nil {
		return nil
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteSnapshot, snapshot.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := snapshot.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshSnapshots()
		})
	}, nil)
}
