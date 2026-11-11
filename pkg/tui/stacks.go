package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
)

// localStackDir: the directory the local stack comes from, and
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

// stackCache: each stack's compose config by directory, read once and
// again when its files change.
type stackCache struct {
	mutex  sync.Mutex
	stacks map[string]cachedStack
}

type cachedStack struct {
	stack *commands.ComposeStack
	stamp string
}

// get is dir's config, loading it if it isn't held, if its files changed
// since - or, with retry, if what's held is a failure.
func (c *stackCache) get(dir string, retry bool, load func(string) *commands.ComposeStack) *commands.ComposeStack {
	// Before the load, so an edit made during it is seen next time.
	stamp := commands.StackFilesStamp(dir)

	c.mutex.Lock()
	cached, ok := c.stacks[dir]
	c.mutex.Unlock()

	if ok && cached.stamp == stamp && (cached.stack.Err == nil || !retry) {
		return cached.stack
	}

	stack := load(dir)
	c.store(stack, stamp)

	return stack
}

// edited is whether any of dirs' files changed since they were read.
func (c *stackCache) edited(dirs []string) bool {
	for _, dir := range dirs {
		c.mutex.Lock()
		cached, ok := c.stacks[dir]
		c.mutex.Unlock()

		if ok && cached.stamp != commands.StackFilesStamp(dir) {
			return true
		}
	}

	return false
}

func (c *stackCache) put(stack *commands.ComposeStack) {
	c.store(stack, commands.StackFilesStamp(stack.Dir))
}

func (c *stackCache) store(stack *commands.ComposeStack, stamp string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stacks == nil {
		c.stacks = map[string]cachedStack{}
	}

	c.stacks[stack.Dir] = cachedStack{stack: stack, stamp: stamp}
}

func (c *stackCache) forget(dir string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.stacks, dir)
}

// fetchStacks reads the stacks' configs, cached, and every compose
// instance's status: one listing a remote, whatever the number of stacks.
func (m *Model) fetchStacks(l *commands.Listing) fetch {
	if m.composeUnavailable() {
		return func() (func(), error) { return func() {}, nil }
	}

	ticket := m.refreshes.stacks.issue()
	cfg, src := m.cfg, m.sources()

	return func() (func(), error) {
		state, err := cfg.LoadAppState()
		if err != nil {
			return nil, err
		}

		stacks := src.listStacks(state.Stacks)
		elsewhere := slices.ContainsFunc(stacks, func(stack *commands.ComposeStack) bool { return !src.onSessionRemote(stack.Remote) })
		here := src.stacksHereIn(stacks)

		if err := src.readStackStatuses(stacks, l); err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.stacks.admit(ticket) {
				m.setStacks(stacks, elsewhere, here)
			}
		}, nil
	}
}

// newStacks: Stacks' fetch and fill are its own, as they span remotes.
func (m *Model) newStacks(p *panel) list[*commands.ComposeStack] {
	return list[*commands.ComposeStack]{
		panel:   p,
		key:     (*commands.ComposeStack).Ref,
		project: func(stack *commands.ComposeStack) string { return stack.Name },
		version: func(stack *commands.ComposeStack) string {
			return stack.Name + "-" + fmt.Sprint(stack.Statuses, stack.Changed()) +
				"-" + fmt.Sprint(stack.StatusErr, stack.StatusPending) + "-" + fmt.Sprint(stack.Local, stack.Saved)
		},
	}
}

// setStacks is the apply of fetchStacks.
func (m *Model) setStacks(stacks []*commands.ComposeStack, elsewhere, here bool) {
	m.stacksElsewhere, m.stacksHere = elsewhere, here
	m.setStackDirs(stacks)
	m.leaveHiddenView()

	sort.SliceStable(stacks, func(a, b int) bool { return sortStacks(stacks[a], stacks[b]) })
	m.refill(m.stacks.panel, m.stacks.keyOf, func() [][]string {
		m.stacks.items = stacks

		return m.stackCells()
	})
	m.setStackServices(stacks)
	m.followStack(m.stacks.selected())

	if m.landing {
		m.landing = false
		m.landOffStacks()
	}
}

// sortStacks: the local stack first when it's saved nowhere,
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
	cells := make([][]string, len(m.stacks.items))
	for i, stack := range m.stacks.items {
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
	m.refill(m.stacks.panel, m.stacks.keyOf, m.stackCells)
}

// setStackServices: the stacks' compose instances, which
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

// setStackDirs: where each listed stack's directory is, and which remotes they're on.
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
