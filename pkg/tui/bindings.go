package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// binding is pkg/gui's Binding: a key on a view, "" for every view.
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

// keyLabels are pkg/gui's, for the keys without a rune it can name.
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

// listed is pkg/gui's: offered by the options menu and the palette.
func (b *binding) listed() bool {
	return b.description != "" && (b.key == "" || b.label() != "")
}

// hasRune is whether gocui binds key by its rune, which a prompt types.
func hasRune(key string) bool {
	return utf8.RuneCountInString(key) == 1
}

func do(f func(*Model)) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		f(m)
		return nil
	}
}

// bindings is pkg/gui's GetInitialKeybindings for what's ported, the top popup's first.
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
	)
	bindings = append(bindings, m.menuBindings()...)
	bindings = append(bindings,
		&binding{view: "instances", key: "E", description: m.tr.ExecShell, mutates: true, handler: (*Model).execShell},
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
		&binding{key: "+", description: m.tr.LcNextScreenMode, handler: do(func(m *Model) { m.nextScreenMode(1) })},
		&binding{key: "_", description: m.tr.LcPrevScreenMode, handler: do(func(m *Model) { m.nextScreenMode(-1) })},
		&binding{key: "=", description: m.tr.LcToggleExpandSidePanel, handler: do(func(m *Model) { m.expandSidePanel = !m.expandSidePanel })},
		&binding{key: "tab", description: m.tr.NextPanel, handler: do(func(m *Model) { m.cycleWindows(1) })},
		&binding{key: "shift+tab", description: m.tr.PrevPanel, handler: do(func(m *Model) { m.cycleWindows(-1) })},
	)

	for _, key := range []string{"left", "h"} {
		bindings = append(bindings, &binding{key: key, description: m.tr.PreviousList, handler: do(func(m *Model) { m.cycleViews(-1) })})
	}
	for _, key := range []string{"right", "l"} {
		bindings = append(bindings, &binding{key: key, description: m.tr.NextList, handler: do(func(m *Model) { m.cycleViews(1) })})
	}

	for index, window := range m.windowNames() {
		bindings = append(bindings, &binding{
			key:         fmt.Sprintf("%d", index+1),
			description: fmt.Sprintf(m.tr.FocusPanel, strings.ToLower(m.windowTitle(window))),
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

// handleKey is gocui's execKeybindings: search keys, the view's binding,
// main's parent's, the editor, then a global one.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if m.searchKey(key) {
		return nil
	}

	view := m.currentView()
	editable := view == filterView
	// An editable view types a rune, so no binding of one matches it.
	typed := editable && hasRune(key)
	// gocui hands an editable view's ctrl+u, ctrl+a and ctrl+e to its editor.
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
			// gocui keeps the last match for the parent.
			parent = b
		case global == nil && b.view == "" && globalOK:
			global = b
		}
	}

	switch {
	case parent != nil:
		return run(parent)
	case editable && isEditorKey(msg):
		return m.editFilter(msg)
	case global != nil:
		return run(global)
	}

	return nil
}

// guardReadOnly is pkg/gui's: b refuses, saying why, on a read-only remote.
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

// actionRemote is pkg/gui's, for the panels ported so far: the session's.
func (m *Model) actionRemote(string) string {
	return m.incus.RemoteName()
}

func (m *Model) isReadOnly(remote string) bool {
	return m.cfg.ReadOnly || m.cfg.UserConfig.Remotes[remote].ReadOnly
}
