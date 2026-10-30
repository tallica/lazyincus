package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
)

// localStackDir is pkg/gui's: the directory the local stack comes from, and
// whether it was named rather than being wherever lazyincus started.
func localStackDir() (string, bool) {
	if dir := os.Getenv("INCUS_COMPOSE_PROJECT_DIRECTORY"); dir != "" {
		if absolute, err := filepath.Abs(dir); err == nil {
			return absolute, true
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	return dir, false
}

// stackCache is pkg/gui's: each stack's compose config by directory, read once.
type stackCache struct {
	mutex  sync.Mutex
	stacks map[string]*commands.ComposeStack
}

// get is dir's config, loading it if it isn't held - or, with retry, if
// what's held is a failure.
func (c *stackCache) get(dir string, retry bool, load func(string) *commands.ComposeStack) *commands.ComposeStack {
	c.mutex.Lock()
	stack, ok := c.stacks[dir]
	c.mutex.Unlock()

	if ok && (stack.Err == nil || !retry) {
		return stack
	}

	stack = load(dir)
	c.put(stack)

	return stack
}

func (c *stackCache) put(stack *commands.ComposeStack) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stacks == nil {
		c.stacks = map[string]*commands.ComposeStack{}
	}

	c.stacks[stack.Dir] = stack
}

func (c *stackCache) forget(dir string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.stacks, dir)
}

// listStacks is pkg/gui's: every stack with its config, each a copy of the cached one.
func (m *Model) listStacks(saved []string) []*commands.ComposeStack {
	stacks := []*commands.ComposeStack{}
	byRef := map[string]*commands.ComposeStack{}

	// Keyed by the remote each is on now, so the local stack and the same
	// directory saved for the session's remote are one row.
	add := func(remote, dir string, local bool) {
		on := remote
		if on == "" {
			on = m.incus.RemoteName()
		}

		ref := commands.StackRef(on, dir)
		if existing, ok := byRef[ref]; ok {
			existing.Local = existing.Local || local
			existing.Saved = existing.Saved || !local

			if !local {
				existing.Remote = remote
			}

			return
		}

		cached := m.stackCache.get(dir, !local || m.localStackExplicit, m.loadStack)
		if local && !m.localStackExplicit && cached.Err != nil {
			return
		}

		stack := *cached
		stack.Remote = remote
		stack.Local, stack.Saved = local, !local
		byRef[ref] = &stack
		stacks = append(stacks, &stack)
	}

	if m.localStackDir != "" {
		add("", m.localStackDir, true)
	}

	for _, ref := range saved {
		remote, dir := commands.ParseStackRef(ref)
		add(remote, dir, false)
	}

	return stacks
}

// fetchStacks reads the stacks' configs, cached, and every compose
// instance's status: one listing a remote, whatever the number of stacks.
func (m *Model) fetchStacks() fetch {
	if m.composeUnavailable() {
		return func() (func(), error) { return func() {}, nil }
	}

	ticket := m.refreshes.stacks.issue()
	cfg := m.cfg

	return func() (func(), error) {
		state, err := cfg.LoadAppState()
		if err != nil {
			return nil, err
		}

		stacks := m.listStacks(state.Stacks)
		elsewhere := slices.ContainsFunc(stacks, func(stack *commands.ComposeStack) bool { return !m.onSessionRemote(stack.Remote) })
		here := m.stacksHereIn(stacks)

		if err := m.readStackStatuses(stacks); err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.stacks.admit(ticket) {
				m.setStacks(stacks, elsewhere, here)
			}
		}, nil
	}
}

func (m *Model) stacksHereIn(stacks []*commands.ComposeStack) bool {
	return slices.ContainsFunc(stacks, func(stack *commands.ComposeStack) bool {
		return m.onSessionRemote(stack.Remote)
	})
}

// readStackStatuses is pkg/gui's: the session's remote asked, every other one's as last read.
func (m *Model) readStackStatuses(stacks []*commands.ComposeStack) error {
	byRemote := lo.GroupBy(stacks, func(stack *commands.ComposeStack) string {
		if stack.Remote == m.incus.RemoteName() {
			return ""
		}

		return stack.Remote
	})

	var sessionErr error

	for remote, remoteStacks := range byRemote {
		var statuses remoteStatuses

		pending := false

		if remote == "" {
			statuses.byProject, statuses.err = m.incus.GetComposeStatuses()
			sessionErr = statuses.err
		} else {
			var read bool

			statuses, read = m.remoteStatuses(remote)
			pending = !read
		}

		for _, stack := range remoteStacks {
			stack.StatusPending = pending
			stack.StatusErr = statuses.err

			if stack.Name != "" {
				project := statuses.byProject[stack.Name]
				stack.Statuses, stack.Images = project.Statuses, project.Images
			}
		}
	}

	return sessionErr
}

// remoteStatuses is another remote's compose statuses as last read, false
// before any has been, asking again in the background.
func (m *Model) remoteStatuses(remote string) (remoteStatuses, bool) {
	read := func() (map[string]commands.ComposeProjectState, error) {
		command, err := m.commandFor(remote)
		if err != nil {
			return nil, err
		}

		return command.GetComposeStatuses()
	}

	return m.remotes.cachedStatuses(remote, read, m.notifyRemotes)
}

// setStacks is the apply of fetchStacks, in pkg/gui's order.
func (m *Model) setStacks(stacks []*commands.ComposeStack, elsewhere, here bool) {
	m.stacksElsewhere, m.stacksHere = elsewhere, here
	m.setStackDirs(stacks)
	m.leaveHiddenView()

	sort.SliceStable(stacks, func(a, b int) bool { return sortStacks(stacks[a], stacks[b]) })
	m.refill(m.panelNamed("stacks"), func(item int) string { return m.stacks[item].Ref() }, func() [][]string {
		m.stacks = stacks

		return m.stackCells()
	})
	m.setStackServices(stacks)
	m.followStack(m.selectedStackRow())

	if m.landing {
		m.landing = false
		m.landOffStacks()
	}
}

// sortStacks is pkg/gui's: the local stack first when it's saved nowhere,
// then each saved remote's together.
func sortStacks(a, b *commands.ComposeStack) bool {
	if a.Saved != b.Saved {
		return !a.Saved
	}

	if a.Remote != b.Remote {
		return a.Remote < b.Remote
	}

	if a.Title() != b.Title() {
		return a.Title() < b.Title()
	}

	return a.Ref() < b.Ref()
}

func (m *Model) stackCells() [][]string {
	cells := make([][]string, len(m.stacks))
	for i, stack := range m.stacks {
		var remote *presentation.StackRemote
		if m.stacksElsewhere {
			remote = &presentation.StackRemote{Name: m.stackRemote(stack), Active: m.onSessionRemote(stack.Remote)}
		}

		cells[i] = presentation.GetStackDisplayStrings(&m.cfg.UserConfig.Gui, stack, m.home, remote, m.upstreams)
	}

	return cells
}

// rerenderStacks shows the stacks' marks, which the registry checks feed.
func (m *Model) rerenderStacks() {
	m.refill(m.panelNamed("stacks"), func(item int) string { return m.stacks[item].Ref() }, m.stackCells)
}

func (m *Model) selectedStackRow() *commands.ComposeStack {
	item := m.panelNamed("stacks").item()
	if item < 0 || item >= len(m.stacks) {
		return nil
	}

	return m.stacks[item]
}

// stackKey is pkg/gui's GetItemContextCacheKey for the selected stack.
func (m *Model) stackKey() string {
	stack := m.selectedStackRow()
	if stack == nil {
		return ""
	}

	return "stacks-" + stack.Ref() + "-" + stack.Name + "-" + fmt.Sprint(stack.Statuses) +
		"-" + fmt.Sprint(stack.StatusErr, stack.StatusPending) + "-" + fmt.Sprint(stack.Local, stack.Saved)
}

// setStackServices is pkg/gui's: the stacks' compose instances, which
// Instances re-filters when they change.
func (m *Model) setStackServices(stacks []*commands.ComposeStack) {
	services := map[string]map[string]bool{}

	for _, stack := range stacks {
		// Another remote's instances were never in the instances panel.
		if stack.Name == "" || !m.onSessionRemote(stack.Remote) {
			continue
		}

		if services[stack.Name] == nil {
			services[stack.Name] = map[string]bool{}
		}

		for _, service := range stack.Services {
			services[stack.Name][service.Name] = true
		}
	}

	if maps.EqualFunc(services, m.stackServices, maps.Equal) {
		return
	}

	m.stackServices = services
	m.rerenderInstances()
}

// stackDirKey is how stackDirs finds a listed stack: by the remote it's on
// and its compose project, which is also its instances' Incus project.
func stackDirKey(remote, project string) string {
	return remote + "\x00" + project
}

// setStackDirs is pkg/gui's: where each listed stack's directory is, and which remotes they're on.
func (m *Model) setStackDirs(stacks []*commands.ComposeStack) {
	dirs := map[string]string{}
	remotes := []string{}

	for _, stack := range stacks {
		if remote := m.stackRemote(stack); !slices.Contains(remotes, remote) {
			remotes = append(remotes, remote)
		}

		if stack.Name == "" {
			continue
		}

		key := stackDirKey(m.stackRemote(stack), stack.Name)
		if _, ok := dirs[key]; !ok {
			dirs[key] = stack.Dir
		}
	}

	m.stackDirs, m.stackRemotes = dirs, remotes
}

// stackLabel is where the stack in project is listed from, or that it isn't.
func (m *Model) stackLabel(remote, project string) string {
	if dir, ok := m.stackDirs[stackDirKey(remote, project)]; ok {
		return commands.ShortenHome(dir, m.home)
	}

	return m.tr.StackNotListed
}
