package tui

import (
	"os"
	"slices"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

func marker(selected bool) string {
	if selected {
		return "*"
	}

	return " "
}

// switchRemoteMenu: the CLI's remotes that
// hold instances, to move every panel onto one for the rest of the session.
func (m *Model) switchRemoteMenu() tea.Cmd {
	current := m.incus.RemoteName()
	names := m.remotes.names()

	items := make([]*menuItem, len(names))
	for i, name := range names {
		items[i] = &menuItem{
			labelColumns: []string{marker(name == current), name},
			onPress:      func(m *Model) tea.Cmd { return m.switchToRemote(name) },
		}
	}

	m.openMenu(menuOptions{title: m.tr.RemotesTitle, items: items, selected: max(slices.Index(names, current), 0)})

	return nil
}

// switchToRemote connects off the loop, and only once that has worked lets
// go of the remote the panels are on: a remote that doesn't answer leaves
// everything where it was.
func (m *Model) switchToRemote(name string) tea.Cmd {
	return m.switchToRemoteThen(name, func() tea.Cmd { return nil })
}

// switchToRemoteThen is switchToRemote, then run once the panels are on the
// remote - straight away when they already are. A project the panels are
// scoped to stays, if the remote has one by that name.
func (m *Model) switchToRemoteThen(name string, then func() tea.Cmd) tea.Cmd {
	if name == m.incus.RemoteName() {
		return then()
	}

	project := ""
	if !m.incus.IsAllProjects() {
		project = m.incus.ProjectName()
	}

	var command *commands.IncusCommand

	return m.withWaitingStatusThen(m.tr.ConnectingStatus, func() error {
		var err error

		command, err = m.remotes.connectNow(name)
		if err != nil {
			return err
		}

		if project != "" {
			if names, err := command.GetProjectNames(); err != nil || !slices.Contains(names, project) {
				project = ""
			}
		}

		return nil
	}, func(err error) tea.Cmd {
		if err != nil {
			m.openError(err.Error())
			return nil
		}

		m.queue(m.moveToRemote(name, command, project))
		m.queue(then())

		return nil
	})
}

// moveToRemote puts every panel on command's remote, scoped to project, or
// to every project when that's empty.
func (m *Model) moveToRemote(name string, command *commands.IncusCommand, project string) tea.Cmd {
	m.incus.UseRemote(command)
	if project != "" {
		m.incus.UseProject(project)
	}

	// From the stacks already listed, rather than once every panel has been
	// read again from the new remote; the stacks read then confirms it.
	if !m.composeUnavailable() {
		m.stacksHere = m.stacksHereIn(m.stacks)
		m.landOffStacks()
		m.leaveHiddenView()
	}

	m.landing = true

	// The shell-outs read it, as they did --remote's.
	if err := os.Setenv("INCUS_REMOTE", name); err != nil {
		m.openError(err.Error())
		return nil
	}

	return m.reloadAfterScopeChange()
}

// projectsMsg is the project names a P asked for.
type projectsMsg struct {
	remote string
	names  []string
	err    error
}

// switchProjectMenu: instances outside the
// remote's configured project are invisible otherwise, which hides every
// incus-compose stack.
func (m *Model) switchProjectMenu() tea.Cmd {
	incus, remote := m.incus, m.incus.RemoteName()

	return func() tea.Msg {
		names, err := incus.GetProjectNames()

		return projectsMsg{remote, names, err}
	}
}

func (m *Model) onProjects(msg projectsMsg) {
	// An R since: these are another remote's projects.
	if msg.remote != m.incus.RemoteName() {
		return
	}

	if msg.err != nil {
		m.handleError(msg.err)
		return
	}

	m.projectsMenu(msg.names)
}

func (m *Model) projectsMenu(names []string) {
	sort.Strings(names)
	current := m.incus.ProjectName()

	items := make([]*menuItem, 0, len(names)+1)
	items = append(items, &menuItem{
		labelColumns: []string{marker(m.incus.IsAllProjects()), m.tr.AllProjects},
		onPress:      func(m *Model) tea.Cmd { return m.switchToAllProjects() },
	})

	for _, name := range names {
		items = append(items, &menuItem{
			labelColumns: []string{marker(name == current), name},
			onPress:      func(m *Model) tea.Cmd { return m.switchToProject(name) },
		})
	}

	selected := 0
	if !m.incus.IsAllProjects() {
		selected = slices.Index(names, current) + 1
	}

	m.openMenu(menuOptions{title: m.tr.ProjectsTitle, items: items, selected: selected})
}

// switchToAllProjects lists every project's instances, images, volumes and
// networks at once; actions still run against the project each item came from.
func (m *Model) switchToAllProjects() tea.Cmd {
	if m.incus.IsAllProjects() {
		return nil
	}

	m.incus.UseAllProjects()

	return m.reloadAfterScopeChange()
}

func (m *Model) switchToProject(name string) tea.Cmd {
	if name == m.incus.ProjectName() {
		return nil
	}

	m.incus.UseProject(name)

	return m.reloadAfterScopeChange()
}

// reloadAfterScopeChange drops what the panels hold of the scope just left,
// so the next refresh can't match new items against stale ones by key, and
// reads everything again.
func (m *Model) reloadAfterScopeChange() tea.Cmd {
	// The stacks are directories, which the read below lists again.
	for _, p := range m.panels {
		if p.name != "stacks" {
			p.awaiting = m.tr.Loading
			p.rowsWidth = -1
			p.cells, p.shown = nil, nil
		}
	}

	m.instances, m.serviceRows, m.snapshots = nil, nil, nil
	m.listed.Store(nil)
	m.images, m.volumes, m.networks, m.profiles = nil, nil, nil, nil
	m.composeState.instances.Store(nil)
	m.composeState.project.Store(nil)
	m.snapshotsFollow = snapshotsFollow{}

	// The blank frame shows "Loading…" in main; without this, the same
	// instance arriving at the top reads as nothing changed.
	m.mainTask.key = ""
	m.jumpToTopMain()
	m.panelNamed("instances").selected = 0

	// The history is the scope's, and a remote's warnings its own.
	m.operations.clear()
	m.operationsSeenAt = time.Now()
	m.countOperations()
	m.warnings, m.newWarnings = nil, 0

	m.refreshes.invalidateAll()

	cmds := []tea.Cmd{m.reloadAll(m.tr.LoadingStatus)}
	if m.streamEvents {
		cmds = append(cmds, m.listen())
	}

	return tea.Batch(cmds...)
}
