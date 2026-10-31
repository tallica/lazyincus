package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// binding is a key on a view, "" for every view.
type binding struct {
	view string
	// key is Bubble Tea's name for it; "" is offered by the palette alone.
	key         string
	description string
	// mutates keys change a daemon: a read-only remote refuses them.
	mutates bool
	// noSelection marks a panel's key that doesn't act on the selected row.
	noSelection bool
	handler     func(*Model) tea.Cmd
}

// keyLabels name the keys that have no rune.
var keyLabels = map[string]string{
	"tab": "tab", "shift+tab": "shift+tab", "esc": "esc", "enter": "enter", "space": "space",
	"right": "►", "left": "◄", "up": "▲", "down": "▼", "pgup": "PgUp", "pgdown": "PgDn", "ctrl+p": "ctrl+p",
}

func (b *binding) label() string {
	if hasRune(b.key) {
		return b.key
	}

	return keyLabels[b.key]
}

// listed is whether the options menu and the palette offer the binding.
func (b *binding) listed() bool {
	return b.description != "" && (b.key == "" || b.label() != "")
}

// hasRune is whether key is a single rune, which a prompt types.
func hasRune(key string) bool {
	return utf8.RuneCountInString(key) == 1
}

func do(f func(*Model)) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		f(m)
		return nil
	}
}

// bindings are every binding in play, the top popup's first.
func (m *Model) bindings() []*binding {
	var bindings []*binding
	if len(m.popups) > 0 {
		bindings = append(bindings, m.popups[len(m.popups)-1].bindings...)
	}

	scrollStep := func(f func(*pane, int)) func(*Model) tea.Cmd {
		return do(func(m *Model) { f(&m.main, m.cfg.UserConfig.Gui.ScrollHeight) })
	}
	scroll := func(direction int) func(*Model) tea.Cmd {
		return do(func(m *Model) { m.scrollMain(direction) })
	}

	bindings = append(bindings,
		&binding{key: "esc", handler: do((*Model).escape)},
		&binding{key: "q", handler: (*Model).quit},
		&binding{key: "ctrl+c", handler: (*Model).quit},
		&binding{key: "pgup", handler: scroll(-1)},
		&binding{key: "pgdown", handler: scroll(1)},
		&binding{key: "ctrl+u", handler: scroll(-1)},
		&binding{key: "ctrl+d", handler: scroll(1)},
		&binding{key: "end", handler: do(func(m *Model) { m.main.autoscroll = true })},
		&binding{key: "home", handler: do((*Model).jumpToTopMain)},
		&binding{key: "x", handler: do((*Model).openOptionsMenu)},
		&binding{key: "?", handler: do((*Model).openOptionsMenu)},
		&binding{key: "ctrl+p", description: m.tr.CommandPalette, handler: do((*Model).openPaletteKey)},
		&binding{key: ":", description: m.tr.TypedCommand, mutates: true, noSelection: true, handler: (*Model).typedCommand},
	)
	bindings = append(bindings, m.menuBindings()...)
	bindings = append(bindings,
		&binding{view: "instances", key: "S", description: m.tr.Start, mutates: true, handler: onInstance((*Model).instanceStart)},
		&binding{view: "instances", key: "s", description: m.tr.Stop, mutates: true, handler: onInstance((*Model).instanceStop)},
		&binding{view: "instances", key: "r", description: m.tr.Restart, mutates: true, handler: onInstance((*Model).instanceRestart)},
		&binding{view: "instances", key: "p", description: m.tr.Pause, mutates: true, handler: onInstance((*Model).instancePauseResume)},
		&binding{view: "instances", key: "d", description: m.tr.Remove, mutates: true, handler: onInstance((*Model).instanceDelete)},
		&binding{view: "instances", key: "e", description: m.tr.HideStopped, noSelection: true, handler: do((*Model).toggleStoppedInstances)},
		&binding{view: "instances", key: "n", description: m.tr.NewSnapshot, mutates: true, handler: onInstance((*Model).snapshotCreatePrompt)},
		&binding{view: "instances", key: "m", description: m.tr.ViewLogs, handler: do((*Model).viewInstanceLogs)},
		&binding{view: "instances", key: "y", description: m.tr.Copy, handler: onInstance((*Model).instanceCopy)},
		&binding{view: "instances", key: "a", description: m.tr.Attach, mutates: true, handler: onInstance((*Model).instanceAttachConsole)},
		&binding{view: "instances", key: "c", description: m.tr.EditInEditor, mutates: true, handler: onInstance((*Model).instanceEdit)},
		&binding{view: "instances", key: "F", description: m.tr.EditFile, mutates: true, handler: onInstance((*Model).instanceEditFile)},
		&binding{view: "instances", description: m.tr.Rename, mutates: true, handler: onInstance((*Model).instanceRename)},
		&binding{view: "instances", description: m.tr.EditDescription, mutates: true, handler: onInstance((*Model).instanceEditDescription)},
		&binding{view: "instances", key: "E", description: m.tr.ExecShell, mutates: true, handler: onInstance((*Model).instanceExecShell)},
		&binding{view: "snapshots", key: "n", description: m.tr.NewSnapshot, mutates: true, noSelection: true, handler: (*Model).snapshotNew},
		&binding{view: "snapshots", key: "e", description: m.tr.ToggleAllSnapshots, noSelection: true, handler: do((*Model).toggleAllSnapshots)},
		&binding{view: "snapshots", key: "r", description: m.tr.RestoreSnapshotShort, mutates: true, handler: onSnapshot((*Model).snapshotRestore)},
		&binding{view: "snapshots", key: "d", description: m.tr.Remove, mutates: true, handler: onSnapshot((*Model).snapshotDelete)},
		&binding{view: "snapshots", description: m.tr.Rename, mutates: true, handler: onSnapshot((*Model).snapshotRename)},
		&binding{view: "snapshots", description: m.tr.NewInstanceFromSnapshot, mutates: true, handler: onSnapshot((*Model).snapshotNewInstance)},
		&binding{view: "snapshots", key: "y", description: m.tr.Copy, handler: onSnapshot((*Model).snapshotCopy)},
		&binding{view: "backups", key: "n", description: m.tr.NewBackup, mutates: true, noSelection: true, handler: (*Model).backupNew},
		&binding{view: "backups", key: "r", description: m.tr.RestoreBackup, mutates: true, handler: onBackup((*Model).backupRestore)},
		&binding{view: "backups", key: "d", description: m.tr.Remove, mutates: true, handler: onBackup((*Model).backupDelete)},
		&binding{view: "backups", key: "D", description: m.tr.PruneBackups, mutates: true, noSelection: true, handler: (*Model).backupPrune},
		&binding{view: "backups", key: "v", description: m.tr.VerifyBackup, handler: onBackup((*Model).backupVerify)},
		&binding{view: "images", key: "d", description: m.tr.Remove, mutates: true, handler: onImage((*Model).imageDelete)},
		&binding{view: "images", key: "c", description: m.tr.EditInEditor, mutates: true, handler: onImage((*Model).imageEdit)},
		&binding{view: "images", key: "u", description: m.tr.ShowUsers, handler: onImage((*Model).imageUsers)},
		&binding{view: "images", key: "D", description: m.tr.PruneImages, mutates: true, noSelection: true, handler: (*Model).pruneImages},
		&binding{view: "images", key: "g", description: m.tr.CheckImageUpdates, noSelection: true, handler: (*Model).checkImageUpdates},
		&binding{view: "images", key: "y", description: m.tr.Copy, handler: onImage((*Model).imageCopy)},
		&binding{view: "networks", key: "c", description: m.tr.EditInEditor, mutates: true, handler: onNetwork((*Model).networkEdit)},
		&binding{view: "networks", key: "u", description: m.tr.ShowUsers, handler: onNetwork((*Model).networkUsers)},
		&binding{view: "networks", key: "e", description: m.tr.HideUnmanagedNetworks, noSelection: true, handler: do((*Model).toggleUnmanagedNetworks)},
		&binding{view: "networks", key: "y", description: m.tr.Copy, handler: onNetwork((*Model).networkCopy)},
		&binding{view: "networks", key: "d", description: m.tr.Remove, mutates: true, handler: onNetwork((*Model).networkDelete)},
		&binding{view: "profiles", key: "u", description: m.tr.ShowUsers, handler: onProfile((*Model).profileUsers)},
		&binding{view: "profiles", key: "c", description: m.tr.EditInEditor, mutates: true, handler: onProfile((*Model).profileEdit)},
		&binding{view: "profiles", key: "d", description: m.tr.Remove, mutates: true, handler: onProfile((*Model).profileDelete)},
		&binding{view: "profiles", key: "y", description: m.tr.Copy, handler: onProfile((*Model).profileCopy)},
		&binding{view: "volumes", key: "n", description: m.tr.NewSnapshot, mutates: true, handler: onVolume((*Model).volumeSnapshotCreatePrompt)},
		&binding{view: "volumes", key: "c", description: m.tr.EditInEditor, mutates: true, handler: onVolume((*Model).volumeEdit)},
		&binding{view: "volumes", key: "u", description: m.tr.ShowUsers, handler: onVolume((*Model).volumeUsers)},
		&binding{view: "volumes", key: "y", description: m.tr.Copy, handler: onVolume((*Model).volumeCopy)},
		&binding{view: "volumes", key: "d", description: m.tr.Remove, mutates: true, handler: onVolume((*Model).volumeDelete)},
		&binding{view: "stacks", key: "a", description: m.tr.AddStack, noSelection: true, handler: (*Model).stackAdd},
		&binding{view: "stacks", key: "D", description: m.tr.RemoveStack, handler: onStackRow((*Model).stackRemove)},
		&binding{view: "stacks", key: "e", description: m.tr.EditStack, handler: onStackRow((*Model).stackEdit)},
		&binding{view: "stacks", key: "c", description: m.tr.EditComposeFile, handler: onStackRow((*Model).stackEditCompose)},
		&binding{view: "stacks", key: "space", description: m.tr.SwitchToStackRemote, handler: onStackRow((*Model).stackSwitchRemote)},
		&binding{view: "stacks", key: "u", description: m.tr.ComposeUp, mutates: true, handler: onStackTarget(composeUp)},
		&binding{view: "stacks", key: "d", description: m.tr.ComposeDown, mutates: true, handler: onStackTarget(composeDownMenu)},
		&binding{view: "stacks", key: "U", description: m.tr.ComposeRecreate, mutates: true, handler: onStackTarget(composeRecreateMenu)},
		&binding{view: "stacks", key: "S", description: m.tr.Start, mutates: true, handler: onStackTarget(composeVerb("", "start"))},
		&binding{view: "stacks", key: "s", description: m.tr.Stop, mutates: true, handler: onStackTarget(composeVerb(m.tr.ConfirmComposeStop, "stop"))},
		&binding{view: "stacks", key: "r", description: m.tr.Restart, mutates: true, handler: onStackTarget(composeVerb("", "restart"))},
		&binding{view: "stacks", key: "p", description: m.tr.Pause, mutates: true, handler: onStack((*Model).stackPause)},
		&binding{view: "stacks", key: "f", description: m.tr.ComposeKill, mutates: true, handler: onStackTarget(composeVerb(m.tr.ConfirmComposeKill, "kill"))},
		&binding{view: "stacks", key: "b", description: m.tr.ComposeBuild, mutates: true, handler: onStackTarget(composeVerb("", "build"))},
		&binding{view: "stacks", key: "g", description: m.tr.ComposePull, mutates: true, handler: onStackTarget(composeVerb("", "pull"))},
		&binding{view: "stacks", key: "G", description: m.tr.ComposeUpdateImages, mutates: true, handler: onStack(updateImages)},
		&binding{view: "stacks", key: "m", description: m.tr.ViewLogs, handler: do((*Model).viewStackLogs)},
		&binding{view: "stacks", key: "M", description: m.tr.ComposeLogs, handler: onStackTarget(composeVerb("", "logs", "--follow"))},
		&binding{view: "stacks", key: "y", description: m.tr.Copy, handler: onStackRow((*Model).stackCopy)},
		&binding{view: "stacks", key: "B", description: m.focusPanelDescription(m.tr.BackupsTitle), handler: (*Model).goToBackups},
		&binding{view: "stacks", key: ";", description: m.tr.TypedComposeCommand, mutates: true, handler: onStackTarget(typedComposeCommand)},
		&binding{view: mainView, key: "esc", description: m.tr.Return, noSelection: true, handler: do((*Model).exitMain)},
		&binding{view: mainView, key: "/", description: m.tr.LcSearch, noSelection: true, handler: do((*Model).openSearch)},
		&binding{view: mainView, key: "left", handler: scrollStep((*pane).scrollLeft)},
		&binding{view: mainView, key: "right", handler: scrollStep((*pane).scrollRight)},
		&binding{view: mainView, key: "h", handler: scrollStep((*pane).scrollLeft)},
		&binding{view: mainView, key: "l", handler: scrollStep((*pane).scrollRight)},
		&binding{view: filterView, key: "enter", handler: (*Model).commitFilter},
		&binding{view: filterView, key: "esc", handler: do((*Model).escapeFilterPrompt)},
		&binding{view: filterView, key: "up", handler: do(func(m *Model) { m.moveFilteredSelection(-1) })},
		&binding{view: filterView, key: "down", handler: do(func(m *Model) { m.moveFilteredSelection(1) })},
		&binding{view: filterView, key: "tab", handler: (*Model).paletteTab},
		&binding{key: "J", handler: scroll(1)},
		&binding{key: "K", handler: scroll(-1)},
		&binding{key: "H", handler: scrollStep((*pane).scrollLeft)},
		&binding{key: "L", handler: scrollStep((*pane).scrollRight)},
		&binding{key: "o", description: m.tr.OpenConfig, handler: (*Model).openConfig},
		&binding{key: "O", description: m.tr.EditConfig, handler: (*Model).editConfig},
		&binding{key: "P", description: m.tr.SwitchProject, handler: (*Model).switchProjectMenu},
		&binding{key: "W", description: m.tr.ShowWarnings, handler: (*Model).openDaemonKey},
		&binding{key: "R", description: m.tr.SwitchRemote, handler: (*Model).switchRemoteMenu},
		&binding{key: "+", description: m.tr.LcNextScreenMode, handler: do(func(m *Model) { m.nextScreenMode(1) })},
		&binding{key: "_", description: m.tr.LcPrevScreenMode, handler: do(func(m *Model) { m.nextScreenMode(-1) })},
		&binding{key: "=", description: m.tr.LcToggleExpandSidePanel, handler: do(func(m *Model) { m.expandSidePanel = !m.expandSidePanel })},
		&binding{key: "tab", description: m.tr.NextPanel, handler: do(func(m *Model) { m.cycleWindows(1) })},
		&binding{key: "shift+tab", description: m.tr.PrevPanel, handler: do(func(m *Model) { m.cycleWindows(-1) })},
	)

	bindings = append(bindings, m.serviceBindings()...)

	for _, key := range []string{"left", "h"} {
		bindings = append(bindings, &binding{key: key, description: m.tr.PreviousList, handler: do(func(m *Model) { m.cycleViews(-1) })})
	}
	for _, key := range []string{"right", "l"} {
		bindings = append(bindings, &binding{key: key, description: m.tr.NextList, handler: do(func(m *Model) { m.cycleViews(1) })})
	}

	if m.composeAvailable {
		bindings = append(bindings, &binding{view: "instances", key: "C", description: m.tr.ToggleStackInstances, noSelection: true, handler: do((*Model).toggleStackInstances)})
	}

	for index, window := range m.windowNames() {
		bindings = append(bindings, &binding{
			key:         fmt.Sprintf("%d", index+1),
			description: m.focusPanelDescription(m.windowTitle(window)),
			handler:     do(func(m *Model) { m.goToWindow(window) }),
		})
	}

	upDown := func(view string, up, down func(*Model) tea.Cmd) {
		bindings = append(bindings,
			&binding{view: view, key: "k", handler: up},
			&binding{view: view, key: "up", handler: up},
			&binding{view: view, key: "j", handler: down},
			&binding{view: view, key: "down", handler: down},
		)
	}
	for _, p := range m.panels {
		upDown(p.name, do(func(m *Model) { m.moveSelection(p, -1) }), do(func(m *Model) { m.moveSelection(p, 1) }))
	}
	menuStep := func(step int) func(*Model) tea.Cmd {
		return do(func(m *Model) {
			if mn := m.topMenu(); mn != nil {
				m.moveSelection(mn.list, step)
			}
		})
	}
	upDown(menuView, menuStep(-1), menuStep(1))
	upDown(mainView, scroll(-1), scroll(1))

	for _, p := range m.panels {
		bindings = append(bindings,
			&binding{view: p.name, key: "enter", description: m.tr.FocusMain, noSelection: true, handler: do((*Model).enterMain)},
			&binding{view: p.name, key: "[", description: m.tr.PreviousContext, noSelection: true, handler: do(func(m *Model) { m.cycleMainTab(p, -1) })},
			&binding{view: p.name, key: "]", description: m.tr.NextContext, noSelection: true, handler: do(func(m *Model) { m.cycleMainTab(p, 1) })},
		)
	}
	for _, p := range m.panels {
		bindings = append(bindings, &binding{
			view: p.name, key: "/", description: m.tr.LcFilter, noSelection: true, handler: do(func(m *Model) { m.openFilter(p) }),
		})
	}

	for _, b := range bindings {
		if b.mutates {
			m.guardReadOnly(b)
		}
	}

	return bindings
}

// handleKey tries search keys, the view's binding,
// main's parent's, the editor, then a global one.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	msg = legacyKey(msg)
	key := msg.String()
	if m.searchKey(key) {
		return nil
	}

	view := m.currentView()
	editable := view == filterView || (view == confirmationView && m.topPrompt() != nil)
	// An editable view types a rune, so no binding of one matches it.
	typed := editable && hasRune(key)
	// An editable view's editor takes ctrl+u, ctrl+a and ctrl+e.
	globalOK := !typed && (!editable || (key != "ctrl+u" && key != "ctrl+a" && key != "ctrl+e"))

	run := func(b *binding) tea.Cmd {
		cmd := b.handler(m)
		m.clampLists()
		return cmd
	}

	var parent, global *binding
	for _, b := range m.bindings() {
		switch {
		case b.key != key || b.handler == nil:
		case b.view == view && !typed:
			return run(b)
		case b.view != "" && b.view == m.mainParent && view == mainView:
			// The last match wins for the parent.
			parent = b
		case global == nil && b.view == "" && globalOK:
			global = b
		}
	}

	switch {
	case parent != nil:
		return run(parent)
	case editable && isEditorKey(msg) && view == filterView:
		return m.editFilter(typedKey(msg))
	case editable && isEditorKey(msg):
		return m.editPrompt(typedKey(msg))
	case global != nil:
		return run(global)
	}

	return nil
}

// guardReadOnly refuses b, saying why, on a read-only remote.
func (m *Model) guardReadOnly(b *binding) {
	handler := b.handler
	b.handler = func(m *Model) tea.Cmd {
		if m.cfg.ReadOnly {
			m.openError(m.tr.ReadOnlySession)
			return nil
		}

		view := b.view
		if view == "" {
			view = m.currentSideView()
		}

		if remote := m.actionRemote(view); m.isReadOnly(remote) {
			m.openError(fmt.Sprintf(m.tr.ReadOnlyRemote, remote))
			return nil
		}

		return handler(m)
	}
}

func (m *Model) focusPanelDescription(title string) string {
	return fmt.Sprintf(m.tr.FocusPanel, strings.ToLower(title))
}

// actionRemote is the remote a key on view acts on.
func (m *Model) actionRemote(view string) string {
	switch view {
	case "snapshots":
		return m.snapshotsRemote()
	case "stacks", "services", "backups":
		if stack := m.selectedStack; stack != nil && stack.Remote != "" {
			return stack.Remote
		}
	}

	return m.incus.RemoteName()
}

func (m *Model) isReadOnly(remote string) bool {
	return m.cfg.ReadOnly || m.cfg.UserConfig.Remotes[remote].ReadOnly
}

// legacyKey folds the ctrl+letter that kitty-protocol terminals report apart
// from tab, enter, esc and backspace back into them, as every other
// terminal reports them.
func legacyKey(msg tea.KeyPressMsg) tea.KeyPressMsg {
	if msg.Mod != tea.ModCtrl {
		return msg
	}

	switch msg.Code {
	case 'i':
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case 'm':
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case '[':
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case 'h':
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}

	return msg
}
