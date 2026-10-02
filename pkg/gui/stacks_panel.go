package gui

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// getStacksPanel lists the compose stacks: the local one, and every one
// saved in state.yml.
func (gui *Gui) getStacksPanel() *panels.SideListPanel[*commands.ComposeStack] {
	return &panels.SideListPanel[*commands.ComposeStack]{
		ContextState: &panels.ContextState[*commands.ComposeStack]{
			GetMainTabs: func() []panels.MainTab[*commands.ComposeStack] {
				return []panels.MainTab[*commands.ComposeStack]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderStackInfo,
					},
					{
						Key:    "logs",
						Title:  gui.Tr.LogsTitle,
						Render: gui.renderStackLogs,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderStackConfig,
					},
				}
			},
			GetItemContextCacheKey: func(stack *commands.ComposeStack) string {
				// fmt prints a map's keys sorted, so the same statuses give
				// the same key.
				return "stacks-" + stack.Ref() + "-" + stack.Name + "-" + fmt.Sprint(stack.Statuses) +
					"-" + fmt.Sprint(stack.StatusErr, stack.StatusPending) + "-" + fmt.Sprint(stack.Local, stack.Saved)
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeStack]{
			List: panels.NewFilteredList[*commands.ComposeStack](),
			View: gui.Views.Stacks,
		},
		NoItemsMessage: gui.Tr.NoStacks,
		Gui:            gui.intoInterface(),
		// A stack hands the snapshots panel nothing, so it empties rather
		// than keep the last list's.
		OnSelect: func(stack *commands.ComposeStack) error {
			if err := gui.refreshSnapshotsFor("", ""); err != nil {
				return err
			}

			return gui.followStack(stack)
		},
		Hide: gui.composeUnavailable,
		// The local stack first when it's saved nowhere, being the one
		// lazyincus was started for, then each saved remote's together - by
		// what's saved alone, since switching remote changes which saved
		// entry the local stack is, and would reshuffle the list under the
		// cursor.
		Sort: func(a, b *commands.ComposeStack) bool {
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
		},
		// Rows are rebuilt on every refresh.
		SameItem: func(a, b *commands.ComposeStack) bool {
			return a.Ref() == b.Ref()
		},
		GetTableCells: func(stack *commands.ComposeStack) []string {
			var remote *presentation.StackRemote
			if gui.State.StacksElsewhere {
				remote = &presentation.StackRemote{Name: gui.stackRemote(stack), Active: gui.onSessionRemote(stack.Remote)}
			}

			return presentation.GetStackDisplayStrings(&gui.Config.UserConfig.Gui, stack, gui.home, remote)
		},
	}
}

// localStackDir is the directory the local stack comes from, and whether it
// was named rather than being wherever lazyincus happened to start: main
// turns -P into incus-compose's own variable, which a user may also set.
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

// stackCache holds each stack's compose config by directory: a subprocess
// each, so read once rather than on every refresh. A compose verb run on a
// stack forgets its entry, so an edited compose file shows after the next.
type stackCache struct {
	mutex  sync.Mutex
	stacks map[string]*commands.ComposeStack
}

// get is dir's config, loading it if it isn't held - or, with retry, if
// what's held is a failure, which a stack that has since gained its compose
// file would otherwise keep.
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

// listStacks is every stack with its config, each a copy of the cached one
// for the statuses to go on. A working directory with no compose file is no
// stack at all, rather than a row saying so: most people start lazyincus
// from somewhere that isn't one.
func (gui *Gui) listStacks(saved []string) []*commands.ComposeStack {
	stacks := []*commands.ComposeStack{}
	byRef := map[string]*commands.ComposeStack{}

	// Keyed by the remote each is on now, so the local stack and the same
	// directory saved for the session's remote are one row, which the
	// saved entry's remote then names.
	add := func(remote, dir string, local bool) {
		on := remote
		if on == "" {
			on = gui.IncusCommand.RemoteName()
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

		cached := gui.stacks.get(dir, !local || gui.localStackExplicit, gui.loadStack)
		if local && !gui.localStackExplicit && cached.Err != nil {
			return
		}

		stack := *cached
		stack.Remote = remote
		stack.Local, stack.Saved = local, !local
		byRef[ref] = &stack
		stacks = append(stacks, &stack)
	}

	if gui.localStackDir != "" {
		add("", gui.localStackDir, true)
	}

	for _, ref := range saved {
		remote, dir := commands.ParseStackRef(ref)
		add(remote, dir, false)
	}

	return stacks
}

// fetchStacks reads the stacks' configs, cached, and every compose
// instance's status: one listing a remote, whatever the number of stacks.
func (gui *Gui) fetchStacks() (func() error, error) {
	if gui.composeUnavailable() {
		return func() error { return nil }, nil
	}

	ticket := gui.refreshes.stacks.issue()

	state, err := gui.Config.LoadAppState()
	if err != nil {
		return nil, err
	}

	stacks := gui.listStacks(state.Stacks)

	elsewhere := slices.ContainsFunc(stacks, func(stack *commands.ComposeStack) bool {
		return !gui.onSessionRemote(stack.Remote)
	})
	gui.stacksElsewhere.Store(elsewhere)

	if err := gui.readStackStatuses(stacks); err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.stacks.admit(ticket) {
			return nil
		}

		gui.State.StacksElsewhere = elsewhere
		gui.setStackDirs(stacks)

		gui.Panels.Stacks.SetItems(stacks)

		if err := gui.Panels.Stacks.RerenderList(); err != nil {
			return err
		}

		if err := gui.setStackServices(stacks); err != nil {
			return err
		}

		// nil when there are none.
		selected, _ := gui.Panels.Stacks.GetSelectedItem()

		return gui.followStack(selected)
	}, nil
}

// readStackStatuses gives each stack its instances' statuses: the
// session's remote asked, every other one's as last read, so a server that
// doesn't answer holds up nothing. A remote that doesn't answer marks its
// own stacks; the session's failing fails the refresh, as every other
// list's does.
func (gui *Gui) readStackStatuses(stacks []*commands.ComposeStack) error {
	byRemote := lo.GroupBy(stacks, func(stack *commands.ComposeStack) string {
		if stack.Remote == gui.IncusCommand.RemoteName() {
			return ""
		}

		return stack.Remote
	})

	var sessionErr error

	for remote, remoteStacks := range byRemote {
		var statuses remoteStatuses

		pending := false

		if remote == "" {
			statuses.byProject, statuses.err = gui.IncusCommand.GetComposeStatuses()
			sessionErr = statuses.err
		} else {
			var read bool

			statuses, read = gui.remoteStatuses(remote)
			pending = !read
		}

		for _, stack := range remoteStacks {
			stack.StatusPending = pending
			stack.StatusErr = statuses.err

			if stack.Name != "" {
				stack.Statuses = statuses.byProject[stack.Name]
			}
		}
	}

	return sessionErr
}

// remoteStatuses is another remote's compose statuses as last read, false
// before any has been, asking again in the background.
func (gui *Gui) remoteStatuses(remote string) (remoteStatuses, bool) {
	read := func() (map[string]map[string][]string, error) {
		command, err := gui.commandFor(remote)
		if err != nil {
			return nil, err
		}

		return command.GetComposeStatuses()
	}

	return gui.remotes.cachedStatuses(remote, read, gui.refreshForRemote)
}

// setStackServices hands the stacks' compose instances to the services
// panel, re-filtering the instances panel when that changes which.
func (gui *Gui) setStackServices(stacks []*commands.ComposeStack) error {
	services := map[string]map[string]bool{}

	for _, stack := range stacks {
		// Another remote's instances were never in the instances panel.
		if stack.Name == "" || !gui.onSessionRemote(stack.Remote) {
			continue
		}

		if services[stack.Name] == nil {
			services[stack.Name] = map[string]bool{}
		}

		for _, service := range stack.Services {
			services[stack.Name][service.Name] = true
		}
	}

	if maps.EqualFunc(services, gui.State.StackServices, maps.Equal) {
		return nil
	}

	gui.State.StackServices = services
	gui.Views.Instances.Title = gui.instancesPanelTitle()
	gui.setInstancesSpan(gui.Panels.Instances.List.GetAllItems())

	return gui.Panels.Instances.RerenderList()
}

func (gui *Gui) refreshStacksQuiet() error {
	if err := gui.refresh(nil, gui.fetchStacks); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// followStack points the services panel at the stack the Stacks panel has
// selected, nil for none. A different stack empties the panel and turns
// away any fetch still out for the old one; its own follows. Main loop
// only.
func (gui *Gui) followStack(stack *commands.ComposeStack) error {
	// Swapped before the invalidation: a fetch whose ticket comes after it
	// has to find the new stack.
	previous := gui.selectedStack.Swap(stack)
	if stackIdentity(previous) == stackIdentity(stack) {
		return nil
	}

	gui.refreshes.services.invalidate()
	gui.composeProject.Store(nil)
	gui.composeInstances.Store(nil)

	gui.Panels.Services.NoItemsMessage = gui.Tr.NoServices
	gui.State.ServicesNote = ""
	if stack == nil {
		gui.Panels.Services.NoItemsMessage = gui.Tr.NoStackSelected
	}

	gui.Panels.Services.SetItems(nil)
	gui.Panels.Services.SetSelectedLineIdx(0)
	gui.Views.Services.Title = gui.servicesPanelTitle()

	if err := gui.Panels.Services.RerenderList(); err != nil {
		return err
	}

	gui.refreshInBackground(gui.fetchServices)

	return nil
}

// stackIdentity is what the services panel's contents depend on: which
// directory on which remote, and the project its compose file names.
func stackIdentity(stack *commands.ComposeStack) string {
	if stack == nil {
		return ""
	}

	return stack.Ref() + "\x00" + stack.Name
}

// renderStackLogs stacks every service's logs, each instance under a
// heading of its own - a replica's name, or a lone instance's service's, as
// Endpoints labels them. `M` is the merged, live view.
func (gui *Gui) renderStackLogs(stack *commands.ComposeStack) tasks.TaskFunc {
	return gui.renderLogsToMain(func() string { return gui.stackLogsStr(stack) })
}

func (gui *Gui) stackLogsStr(stack *commands.ComposeStack) string {
	if stack.Err != nil {
		return utils.ColoredString(stack.Err.Error(), color.FgRed)
	}

	state := gui.composeInstances.Load()
	if !state.isOf(stack) {
		return ""
	}

	var sections []string

	for _, service := range sortedServices(state.services) {
		instances := service.SortedInstances()

		for _, instance := range instances {
			heading := service.Name
			if len(instances) > 1 {
				heading = instance.Name
			}

			sections = append(sections, gui.sectionHeading(heading)+"\n\n"+strings.TrimRight(gui.instanceLogStr(instance), "\n"))
		}
	}

	if len(sections) == 0 {
		return gui.Tr.StackNotRunning
	}

	return strings.Join(sections, "\n\n")
}

func (gui *Gui) handleStackViewLogs(g *gocui.Gui, v *gocui.View) error {
	if err := gui.Panels.Stacks.SetMainTab("logs"); err != nil {
		return err
	}

	return gui.handleEnterMain(g, v)
}

func (gui *Gui) renderStackConfig(stack *commands.ComposeStack) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		config, err := gui.IncusCommand.ComposeStackConfig(stack.Dir)
		if err != nil {
			return fmt.Sprintf("Error running `incus-compose config`: %v", err)
		}

		return utils.ColoredYamlString(config)
	})
}

// handleStackAdd is `a`: a directory to list, saved in state.yml with the
// remote it's pinned to.
func (gui *Gui) handleStackAdd(g *gocui.Gui, v *gocui.View) error {
	return gui.openTextPrompt(gui.Tr.AddStackPrompt, gui.Tr.AddStackHint, "", func(input string) error {
		return gui.WithWaitingStatus(gui.Tr.AddingStackStatus, func() error {
			return gui.addStack(input)
		})
	})
}

// addStack takes a directory only once incus-compose has read a compose
// project from it: a stack that's never going to work is a row of noise.
// A remote that doesn't answer is no reason to refuse, being the remote's
// to fix. Off the main loop.
func (gui *Gui) addStack(input string) error {
	return gui.saveStack(input, "")
}

// stackEdit is `e`: the stack's entry in the add prompt, to move it
// to another directory or remote. Saved in place, so the list keeps its
// order in state.yml.
func (gui *Gui) stackEdit(stack *commands.ComposeStack) error {
	if !stack.Saved {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.CannotEditLocalStack, presentation.StackPath(stack, gui.home)))
	}

	initial := commands.StackRef(stack.Remote, commands.ShortenHome(stack.Dir, gui.home))

	return gui.openTextPrompt(gui.Tr.EditStackPrompt, gui.Tr.EditStackHint, initial, func(input string) error {
		return gui.WithWaitingStatus(gui.Tr.SavingStatus, func() error {
			return gui.saveStack(input, stack.Ref())
		})
	})
}

// saveStack is addStack, or with replacing the entry what was typed takes
// the place of.
func (gui *Gui) saveStack(input, replacing string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	remote, path := commands.SplitStackInput(input, func(name string) bool {
		return name == gui.IncusCommand.RemoteName() || gui.remotes.known(name)
	})
	// Pinned to where it was added, so a session on another remote never
	// runs its verbs there.
	if remote == "" {
		remote = gui.IncusCommand.RemoteName()
	}

	dir, err := commands.ResolveStackDir(path, cwd, gui.home)
	if err != nil {
		return err
	}

	ref := commands.StackRef(remote, dir)
	if ref == replacing {
		return nil
	}

	if err := commands.CheckStackDir(dir); err != nil {
		return err
	}

	state, err := gui.Config.LoadAppState()
	if err != nil {
		return err
	}

	if slices.Contains(state.Stacks, ref) || (gui.onSessionRemote(remote) && gui.isLocalStack(dir)) {
		return fmt.Errorf(gui.Tr.StackAlreadyListed, ref)
	}

	stack := gui.loadStack(dir)
	if stack.Err != nil {
		return fmt.Errorf(gui.Tr.StackNotComposeProject, dir, stack.Err)
	}

	save := gui.Config.AddStack
	if replacing != "" {
		save = func(ref string) error { return gui.Config.ReplaceStack(replacing, ref) }
	}

	if err := save(ref); err != nil {
		if errors.Is(err, config.ErrStackListed) {
			return fmt.Errorf(gui.Tr.StackAlreadyListed, ref)
		}

		return err
	}

	gui.stacks.put(stack)

	return gui.refresh(func() error { return gui.selectStack(ref) }, gui.fetchStacks)
}

// stackRemote is the remote a stack's verbs go to: its own, or the
// session's for one that follows it.
func (gui *Gui) stackRemote(stack *commands.ComposeStack) string {
	if stack.Remote == "" {
		return gui.IncusCommand.RemoteName()
	}

	return stack.Remote
}

// onSessionRemote is whether a stack pinned to remote is on the remote the
// rest of the panels show.
func (gui *Gui) onSessionRemote(remote string) bool {
	return remote == "" || remote == gui.IncusCommand.RemoteName()
}

// isLocalStack is whether dir is the local stack's, listed.
func (gui *Gui) isLocalStack(dir string) bool {
	if dir != gui.localStackDir {
		return false
	}

	return gui.localStackExplicit || gui.stacks.get(dir, false, gui.loadStack).Err == nil
}

// selectStack moves the Stacks panel's cursor to ref's row, and the focus
// to the panel.
func (gui *Gui) selectStack(ref string) error {
	index := gui.Panels.Stacks.List.GetIndexBy(func(stack *commands.ComposeStack) bool { return stack.Ref() == ref })
	if index < 0 {
		return nil
	}

	gui.Panels.Stacks.SetSelectedLineIdx(index)

	if err := gui.switchFocus(gui.Views.Stacks); err != nil {
		return err
	}

	return gui.Panels.Stacks.RerenderList()
}

// stackEditCompose is `c`: the stack's compose file in the user's editor,
// read again once it closes.
func (gui *Gui) stackEditCompose(stack *commands.ComposeStack) error {
	file, err := commands.ComposeFile(stack.Dir)
	if err != nil {
		return gui.createErrorPanel(err.Error())
	}

	if err := gui.editFile(file); err != nil {
		return err
	}

	gui.stacks.forget(stack.Dir)
	gui.refreshInBackground(gui.fetchStacks, gui.fetchServices)

	return nil
}

// stackRemove is `D`: forget a saved stack, after asking. Nothing on the
// daemon changes, and the local stack isn't saved to be removed.
func (gui *Gui) stackRemove(stack *commands.ComposeStack) error {
	path := presentation.StackPath(stack, gui.home)

	if !stack.Saved {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.CannotRemoveLocalStack, path))
	}

	// Also the local stack, it keeps its row: only the saved entry goes.
	prompt := gui.Tr.ConfirmRemoveStack
	if stack.Local {
		prompt = gui.Tr.ConfirmRemoveSavedLocalStack
	}

	message := fmt.Sprintf(prompt, stack.Title(), path)

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := gui.Config.RemoveStack(stack.Ref()); err != nil {
				return err
			}

			return gui.refresh(nil, gui.fetchStacks)
		})
	}, nil)
}
