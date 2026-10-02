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
				return "snapshots-" + snapshot.Key() + "-" + utils.Fingerprint(snapshot.Details())
			},
		},
		ListPanel: panels.ListPanel[*commands.Snapshot]{
			List: panels.NewFilteredList[*commands.Snapshot](),
			View: gui.Views.Snapshots,
		},
		NoItemsMessage: gui.Tr.NoSnapshots,
		Gui:            gui.intoInterface(),
		SameItem: func(a, b *commands.Snapshot) bool {
			return a.Key() == b.Key()
		},
		Sort: func(a *commands.Snapshot, b *commands.Snapshot) bool {
			// A service's replicas are snapshotted alike, so their
			// snapshots group by instance rather than interleaving by date;
			// within an instance, newest first: a rollback almost always
			// means the last one.
			if a.Project != b.Project {
				return a.Project < b.Project
			}

			if a.Owner != b.Owner {
				return a.Owner < b.Owner
			}

			return a.CreatedAt().After(b.CreatedAt())
		},
		GetTableCells: func(snapshot *commands.Snapshot) []string {
			return presentation.GetSnapshotDisplayStrings(snapshot, gui.snapshotOwner(snapshot))
		},
		// The instance gives way first: rows group by it, so it repeats down
		// the list, where the name is what you act on.
		FlexColumns: func() []utils.FlexColumn {
			name := utils.FlexColumn{Index: 0, MinWidth: presentation.MinSnapshotNameWidth}
			if !gui.State.SnapshotsSpan.Instances {
				return []utils.FlexColumn{name}
			}

			return []utils.FlexColumn{{Index: 1, MinWidth: presentation.MinSnapshotOwnerWidth}, name}
		},
	}
}

func (gui *Gui) renderSnapshotConfig(snapshot *commands.Snapshot) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.snapshotConfigStr(snapshot) })
}

func (gui *Gui) snapshotConfigStr(snapshot *commands.Snapshot) string {
	padding := 12
	output := ""
	if snapshot.Volume != nil {
		output += utils.WithPadding("Volume: ", padding) + snapshot.Owner + " (" + snapshot.Volume.Pool + ")\n"
	} else {
		output += utils.WithPadding("Instance: ", padding) + snapshot.Owner + "\n"
	}

	output += utils.WithPadding("Name: ", padding) + snapshot.Name + "\n"
	output += utils.WithPadding("Taken at: ", padding) + snapshot.CreatedAt().String() + "\n"

	if snapshot.Volume == nil {
		output += utils.WithPadding("Stateful: ", padding) + fmt.Sprint(snapshot.IsStateful()) + "\n"
	}

	data, err := utils.MarshalIntoYaml(snapshot.Details())
	if err != nil {
		return fmt.Sprintf("Error marshalling snapshot details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

// renderSnapshots lists the snapshots of what the panel follows, as the
// newest refresh has them: they come with the instance and volume listings,
// so neither a selection change nor a poll asks the daemon for anything.
func (gui *Gui) renderSnapshots() error {
	label, instances := gui.snapshotsSource()
	gui.setSnapshotsTitle(label)

	if volume, ok := gui.snapshotsVolume(); ok {
		gui.State.SnapshotsSpan = snapshotsSpan{}
		gui.Panels.Snapshots.SetItems(volume.Snapshots())

		return gui.Panels.Snapshots.RerenderList()
	}

	gui.State.SnapshotsSpan = snapshotsSpan{
		Instances: len(instances) > 1,
		Projects: spansMultipleProjects(lo.Map(instances, func(instance *commands.Instance, _ int) string {
			return instance.Project
		})),
	}

	snapshots := []*commands.Snapshot{}

	for _, instance := range instances {
		snapshots = append(snapshots, instance.Latest().Snapshots()...)
	}

	gui.Panels.Snapshots.SetItems(snapshots)

	return gui.Panels.Snapshots.RerenderList()
}

// refreshSnapshotsFor points the panel at what the selection stands for -
// one instance, or every replica of a service. The panel follows whichever
// list you're in, so each of those hands its own selection over rather than
// the snapshots panel reaching for the focused view - reading that takes
// ViewStackMutex, which switchFocus is holding when it runs a panel's
// OnSelect.
func (gui *Gui) refreshSnapshotsFor(label string, instances ...*commands.Instance) error {
	gui.State.SnapshotsLabel = label
	gui.State.SnapshotsInstances = instances
	gui.State.SnapshotsVolume = ""

	if gui.State.SnapshotsShowAll {
		return nil
	}

	return gui.renderSnapshots()
}

// refreshSnapshotsForVolume points the panel at a custom volume, the volumes
// panel's selection. Held by key: each volumes refresh builds new values.
func (gui *Gui) refreshSnapshotsForVolume(volume *commands.Volume) error {
	gui.State.SnapshotsLabel = volume.Name
	gui.State.SnapshotsInstances = nil
	gui.State.SnapshotsVolume = volume.Key()

	if gui.State.SnapshotsShowAll {
		return nil
	}

	return gui.renderSnapshots()
}

// snapshotsVolume is the volume the panel follows, as the newest refresh
// has it; none while it lists every instance's.
func (gui *Gui) snapshotsVolume() (*commands.Volume, bool) {
	if gui.State.SnapshotsShowAll || gui.State.SnapshotsVolume == "" {
		return nil, false
	}

	return lo.Find(gui.Panels.Volumes.List.GetAllItems(), func(volume *commands.Volume) bool {
		return volume.Key() == gui.State.SnapshotsVolume
	})
}

// snapshotsSource is what the panel lists and what its title calls it:
// every instance's snapshots, the local stack's included, or the selection's.
func (gui *Gui) snapshotsSource() (string, []*commands.Instance) {
	if gui.State.SnapshotsShowAll {
		return gui.Tr.AllSnapshotsLabel, gui.Panels.Instances.List.GetAllItems()
	}

	return gui.State.SnapshotsLabel, gui.State.SnapshotsInstances
}

func (gui *Gui) handleToggleAllSnapshots(g *gocui.Gui, v *gocui.View) error {
	gui.State.SnapshotsShowAll = !gui.State.SnapshotsShowAll

	return gui.renderSnapshots()
}

// snapshotOwner names the instance a row came from when the panel holds more
// than one instance's snapshots, and its project too when those span
// projects, where instance names can repeat.
func (gui *Gui) snapshotOwner(snapshot *commands.Snapshot) string {
	switch {
	case gui.State.SnapshotsSpan.Projects:
		return snapshot.Project + "/" + snapshot.Owner
	case gui.State.SnapshotsSpan.Instances:
		return snapshot.Owner
	default:
		return ""
	}
}

// handleSnapshotCreate snapshots the instance the selected row belongs to
// while the panel lists every instance's, the volume it follows, or the
// instances panel's selection.
func (gui *Gui) handleSnapshotCreate(g *gocui.Gui, v *gocui.View) error {
	if volume, ok := gui.snapshotsVolume(); ok {
		return gui.volumeSnapshotCreatePrompt(volume)
	}

	if gui.State.SnapshotsShowAll {
		if snapshot, err := gui.Panels.Snapshots.GetSelectedItem(); err == nil {
			if instance, ok := lo.Find(gui.Panels.Instances.List.GetAllItems(), func(instance *commands.Instance) bool {
				return instance.Project == snapshot.Project && instance.Name == snapshot.Owner
			}); ok {
				return gui.snapshotCreatePrompt(instance)
			}
		}
	}

	return onSelected(gui.Panels.Instances, gui.snapshotCreatePrompt)(g, v)
}

// setSnapshotsTitle names what the panel is showing, since the list alone
// gives no clue which instance - or service - these snapshots belong to.
func (gui *Gui) setSnapshotsTitle(label string) {
	title := gui.Tr.SnapshotsTitle
	if label != "" {
		title += " (" + label + ")"
	}

	gui.Views.Snapshots.Title = title
}

// snapshotPrompt is the state behind the new-snapshot popup: a name field,
// and an options box holding the rest of what `incus snapshot create` takes.
// The options are fields rather than actions - a row shows a value you
// change in place, and enter always means create, wherever the focus is.
type snapshotPrompt struct {
	// label names what's being snapshotted; create takes the snapshot.
	label  string
	create func(name string, opts commands.SnapshotOptions) error
	// statefulField offers stateful, which only an instance has.
	statefulField bool

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
)

func (p *snapshotPrompt) fieldCount() int {
	if p.statefulField {
		return 2
	}

	return 1
}

func (p *snapshotPrompt) moveField(delta int) {
	p.field = (p.field + delta + p.fieldCount()) % p.fieldCount()
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

func (gui *Gui) snapshotCreatePrompt(instance *commands.Instance) error {
	return gui.openSnapshotPrompt(&snapshotPrompt{
		label:         instance.Name,
		statefulField: true,
		create: func(name string, opts commands.SnapshotOptions) error {
			return gui.createSnapshot(instance, name, opts)
		},
	})
}

// volumeSnapshotCreatePrompt is the same popup for a custom volume; the
// others' snapshots are their instance's.
func (gui *Gui) volumeSnapshotCreatePrompt(volume *commands.Volume) error {
	if !volume.IsCustom() {
		return gui.createErrorPanel(gui.Tr.CannotSnapshotInstanceVolume)
	}

	return gui.openSnapshotPrompt(&snapshotPrompt{
		label: volume.Name,
		create: func(name string, opts commands.SnapshotOptions) error {
			return gui.createVolumeSnapshot(volume, name, opts)
		},
	})
}

func (gui *Gui) openSnapshotPrompt(prompt *snapshotPrompt) error {
	gui.onNewPopupPanel()

	if err := gui.prepareConfirmationPanel(fmt.Sprintf(gui.Tr.SnapshotNamePrompt, prompt.label), ""); err != nil {
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

	return prompt.create(name, prompt.options())
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
	}[:prompt.fieldCount()]

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
	view.SetCursor(0, prompt.field)
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

		return gui.refresh(func() error {
			// Points the panel at this instance alone: taken from a
			// replicated service, the one just picked is one of several it
			// was showing. A no-op while the panel lists every instance's.
			if err := gui.refreshSnapshotsFor(instance.Name, instance); err != nil {
				return err
			}

			return gui.focusSnapshot((&commands.Snapshot{Project: instance.Project, Owner: instance.Name, Name: name}).Key())
		}, gui.fetchInstances, gui.fetchServices)
	})
}

func (gui *Gui) createVolumeSnapshot(volume *commands.Volume, name string, opts commands.SnapshotOptions) error {
	return gui.WithWaitingStatus(gui.Tr.SnapshottingStatus, func() error {
		if err := volume.CreateSnapshot(name, opts); err != nil {
			return gui.createErrorPanel(err.Error())
		}

		return gui.refresh(func() error {
			if err := gui.refreshSnapshotsForVolume(volume); err != nil {
				return err
			}

			return gui.focusSnapshot((&commands.Snapshot{Volume: volume, Name: name}).Key())
		}, gui.fetchVolumes)
	})
}

// focusSnapshot moves to the snapshots panel and puts the cursor on the
// named snapshot, so a snapshot taken from the instances panel lands you
// where you can see it.
func (gui *Gui) focusSnapshot(key string) error {
	index := gui.Panels.Snapshots.List.GetIndexBy(func(snapshot *commands.Snapshot) bool {
		return snapshot.Key() == key
	})
	if index < 0 {
		return nil
	}

	gui.Panels.Snapshots.SetSelectedLineIdx(index)

	return gui.switchFocus(gui.Views.Snapshots)
}

func (gui *Gui) snapshotRestore(snapshot *commands.Snapshot) error {
	prompt := fmt.Sprintf(gui.Tr.RestoreSnapshot, gui.snapshotOwnerName(snapshot), snapshot.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RestoringStatus, func() error {
			// Incus restarts a running instance to restore it; see
			// docs/Incus.md, "Status mid-action".
			if snapshot.Instance != nil {
				return gui.inTransition(snapshot.Instance, "Restoring", snapshot.Restore)
			}

			if err := snapshot.Restore(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshSnapshotOwner(snapshot)
		})
	}, nil)
}

func (gui *Gui) snapshotDelete(snapshot *commands.Snapshot) error {
	// Named with its instance, the panel holding every replica's snapshots
	// where a service is selected, and replicas sharing snapshot names.
	prompt := fmt.Sprintf(gui.Tr.DeleteSnapshot, snapshot.Name, gui.snapshotOwnerName(snapshot))

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := snapshot.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshSnapshotOwner(snapshot)
		})
	}, nil)
}

// snapshotOwnerName is what the snapshot was taken of, for a prompt: with
// its project where the list it came from spans several.
func (gui *Gui) snapshotOwnerName(snapshot *commands.Snapshot) string {
	spans := gui.State.SpansProjects.Instances || gui.State.SnapshotsSpan.Projects
	if snapshot.Volume != nil {
		spans = gui.State.SpansProjects.Volumes
	}

	owner := gui.qualified(snapshot.Owner, snapshot.Project, spans)
	if snapshot.Instance != nil {
		owner = gui.onRemote(owner, snapshot.Instance.Remote)
	}

	return owner
}

// refreshSnapshotOwner re-lists what a snapshot was taken of, which is what
// carries its snapshots.
func (gui *Gui) refreshSnapshotOwner(snapshot *commands.Snapshot) error {
	if snapshot.Volume != nil {
		return gui.refreshVolumes()
	}

	return gui.refreshInstancesAndServices()
}
