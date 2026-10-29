package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

const snapshotOptionsView = "snapshotOptions"

// snapshotPrompt is pkg/gui's: a name field and an options box, enter creating from either.
type snapshotPrompt struct {
	create        func(m *Model, name string, opts commands.SnapshotOptions) tea.Cmd
	statefulField bool

	expiry   int
	stateful bool
	field    int
}

var snapshotExpiries = []struct {
	label string
	value time.Duration
	never bool
}{
	{"default", 0, false},
	{"never", 0, true},
	{"1 day", 24 * time.Hour, false},
	{"7 days", 7 * 24 * time.Hour, false},
	{"30 days", 30 * 24 * time.Hour, false},
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
	p.field = wrap(p.field+delta, p.fieldCount())
}

// changeField cycles the field's value; both wrap, so neither key has a dead end.
func (p *snapshotPrompt) changeField(delta int) {
	switch p.field {
	case snapshotFieldExpiry:
		p.expiry = wrap(p.expiry+delta, len(snapshotExpiries))
	case snapshotFieldStateful:
		p.stateful = !p.stateful
	}
}

func (p *snapshotPrompt) options() commands.SnapshotOptions {
	return commands.SnapshotOptions{
		Stateful:  p.stateful,
		ExpiresIn: snapshotExpiries[p.expiry].value,
		NoExpiry:  snapshotExpiries[p.expiry].never,
	}
}

func (m *Model) snapshotCreatePrompt(instance *commands.Instance) tea.Cmd {
	m.openSnapshotPrompt(m.onRemote(instance.Name, instance.Remote), &snapshotPrompt{
		statefulField: true,
		create: func(m *Model, name string, opts commands.SnapshotOptions) tea.Cmd {
			return m.createSnapshot(instance, name, opts)
		},
	})

	return nil
}

// volumeSnapshotCreatePrompt is pkg/gui's: the same popup for a custom volume.
func (m *Model) volumeSnapshotCreatePrompt(volume *commands.Volume) tea.Cmd {
	if !volume.IsCustom() {
		m.openError(m.tr.CannotSnapshotInstanceVolume)
		return nil
	}

	m.openSnapshotPrompt(volume.Name, &snapshotPrompt{
		create: func(m *Model, name string, opts commands.SnapshotOptions) tea.Cmd {
			return m.createVolumeSnapshot(volume, name, opts)
		},
	})

	return nil
}

// openSnapshotPrompt is pkg/gui's; tab switches popup.view, which the keys follow.
func (m *Model) openSnapshotPrompt(label string, sp *snapshotPrompt) {
	input := newEditorInput()
	p := &popup{
		view: confirmationView, title: fmt.Sprintf(m.tr.SnapshotNamePrompt, label),
		subtitle: m.tr.SnapshotSwitchFocusHint, input: &input, snapshot: sp,
	}

	create := func(m *Model) tea.Cmd {
		name := strings.TrimSpace(p.input.Value())
		if name == "" {
			p.view = confirmationView
			return nil
		}

		m.closePopup()

		return sp.create(m, name, sp.options())
	}
	focus := func(view string) func(*Model) tea.Cmd {
		return do(func(*Model) { p.view = view })
	}
	move := func(delta int) func(*Model) tea.Cmd {
		return do(func(*Model) { sp.moveField(delta) })
	}
	change := func(delta int) func(*Model) tea.Cmd {
		return do(func(*Model) { sp.changeField(delta) })
	}
	closeIt := do((*Model).closePopup)

	for _, b := range []struct {
		view, key string
		handler   func(*Model) tea.Cmd
	}{
		{confirmationView, "enter", create},
		{confirmationView, "ctrl+s", create},
		{confirmationView, "esc", closeIt},
		{confirmationView, "tab", focus(snapshotOptionsView)},
		{snapshotOptionsView, "enter", create},
		{snapshotOptionsView, "ctrl+s", create},
		{snapshotOptionsView, "esc", closeIt},
		{snapshotOptionsView, "down", move(1)},
		{snapshotOptionsView, "up", move(-1)},
		{snapshotOptionsView, "right", change(1)},
		{snapshotOptionsView, "left", change(-1)},
		{snapshotOptionsView, "space", change(1)},
		{snapshotOptionsView, "tab", focus(confirmationView)},
		// Letters only here, where nothing is typed; q so quit can't fire from a modal.
		{snapshotOptionsView, "j", move(1)},
		{snapshotOptionsView, "k", move(-1)},
		{snapshotOptionsView, "l", change(1)},
		{snapshotOptionsView, "h", change(-1)},
		{snapshotOptionsView, "q", closeIt},
	} {
		p.bindings = append(p.bindings, &binding{view: b.view, key: b.key, handler: b.handler})
	}

	m.popups = append(m.popups, p)
}

// snapshotOptionsLines is pkg/gui's renderSnapshotOptions' text.
func (m *Model) snapshotOptionsLines(sp *snapshotPrompt) []string {
	rows := [][2]string{
		{m.tr.SnapshotExpiryField, snapshotExpiries[sp.expiry].label},
		{m.tr.SnapshotStatefulField, m.yesNo(sp.stateful)},
	}[:sp.fieldCount()]

	lines := make([]string, len(rows))
	for i, row := range rows {
		value := "  " + row[1] + "  "
		if i == sp.field {
			value = "‹ " + row[1] + " ›"
		}
		lines[i] = " " + utils.WithPadding(row[0], 12) + value
	}

	return lines
}

func (m *Model) yesNo(value bool) string {
	if value {
		return m.tr.Yes
	}

	return m.tr.No
}

// snapshotOptionsFrame is the options box, drawn from x0 to x1 under the name field.
func (m *Model) snapshotOptionsFrame(p *popup, x0, x1 int) string {
	lines := m.snapshotOptionsLines(p.snapshot)
	w := x1 - x0 + 1
	body := make([]string, len(lines))
	for i, line := range lines {
		body[i] = fit(line, w-2)
		if i == p.snapshot.field {
			body[i] = highlight(line, w-2, m.theme.selectedBg)
		}
	}

	spec := frameSpec{
		title: m.tr.SnapshotOptionsTitle, subtitle: m.tr.SnapshotChangeHint, footer: m.tr.SnapshotSubmitHint,
		body: body, focused: p.view == snapshotOptionsView, total: len(lines),
	}

	return m.theme.frame(spec, w, len(lines)+2)
}

// snapshotOptionsKeys is pkg/gui's renderSnapshotOptionsKeys.
func (m *Model) snapshotOptionsKeys() map[string]string {
	return map[string]string{
		"esc":   m.tr.Close,
		"↑ ↓":   m.tr.Navigate,
		"← →":   m.tr.SnapshotChangeValue,
		"tab":   m.tr.SnapshotFocusName,
		"enter": m.tr.SnapshotCreate,
	}
}

// createSnapshot is pkg/gui's: then the panel on that instance, the cursor on the snapshot.
func (m *Model) createSnapshot(instance *commands.Instance, name string, opts commands.SnapshotOptions) tea.Cmd {
	key := (&commands.Snapshot{Project: instance.Project, Owner: instance.Name, Name: name}).Key()

	return m.withWaitingStatusThen(m.tr.SnapshottingStatus, func() error { return instance.CreateSnapshot(name, opts) },
		m.reportOr(func() tea.Cmd {
			return refresh(append(m.instanceFetches(), func() (func(), error) {
				return func() {
					m.followSnapshotsOf(snapshotsFollow{label: m.onRemote(instance.Name, instance.Remote), instances: []*commands.Instance{instance}})
					m.focusSnapshot(key)
				}, nil
			})...)
		}))
}

func (m *Model) createVolumeSnapshot(volume *commands.Volume, name string, opts commands.SnapshotOptions) tea.Cmd {
	key := (&commands.Snapshot{Volume: volume, Name: name}).Key()

	return m.withWaitingStatusThen(m.tr.SnapshottingStatus, func() error { return volume.CreateSnapshot(name, opts) },
		m.reportOr(func() tea.Cmd {
			return refresh(m.fetchVolumes(), func() (func(), error) {
				return func() {
					m.followSnapshotsOf(snapshotsFollow{label: volume.Name, volume: volume.Key()})
					m.focusSnapshot(key)
				}, nil
			})
		}))
}

// focusSnapshot is pkg/gui's: the cursor on it in a focused Snapshots.
func (m *Model) focusSnapshot(key string) {
	if m.selectSnapshot(key) {
		m.focusView("snapshots")
	}
}

// snapshotNew is pkg/gui's handleSnapshotCreate.
func (m *Model) snapshotNew() tea.Cmd {
	if volume := m.snapshotsVolume(); volume != nil {
		return m.volumeSnapshotCreatePrompt(volume)
	}

	if m.snapshotsShowAll {
		if snapshot := m.selectedSnapshot(); snapshot != nil && snapshot.Instance != nil {
			return m.snapshotCreatePrompt(snapshot.Instance)
		}

		return onInstance((*Model).snapshotCreatePrompt)(m)
	}

	instances := m.snapshotsFollow.instances
	switch len(instances) {
	case 0:
		return nil
	case 1:
		return m.snapshotCreatePrompt(instances[0])
	}

	items := make([]*menuItem, len(instances))
	for i, instance := range instances {
		items[i] = &menuItem{label: instance.Name, onPress: func(m *Model) tea.Cmd { return m.snapshotCreatePrompt(instance) }}
	}
	m.openMenu(menuOptions{title: m.tr.NewSnapshot, items: items})

	return nil
}
