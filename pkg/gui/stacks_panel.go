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
				return "stacks-" + stack.Dir + "-" + stack.Name + "-" + fmt.Sprint(stack.Statuses)
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeStack]{
			List: panels.NewFilteredList[*commands.ComposeStack](),
			View: gui.Views.Stacks,
		},
		NoItemsMessage: gui.Tr.NoStacks,
		Gui:            gui.intoInterface(),
		OnSelect: func(stack *commands.ComposeStack) error {
			return gui.followStack(stack)
		},
		Hide: gui.composeUnavailable,
		// The local stack first, being the one lazyincus was started for.
		Sort: func(a, b *commands.ComposeStack) bool {
			if a.Local != b.Local {
				return a.Local
			}

			if a.Title() != b.Title() {
				return a.Title() < b.Title()
			}

			return a.Dir < b.Dir
		},
		// Rows are rebuilt on every refresh.
		SameItem: func(a, b *commands.ComposeStack) bool {
			return a.Dir == b.Dir
		},
		GetTableCells: func(stack *commands.ComposeStack) []string {
			return presentation.GetStackDisplayStrings(&gui.Config.UserConfig.Gui, stack, gui.home)
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
	byDir := map[string]*commands.ComposeStack{}

	add := func(dir string, local bool) {
		if existing, ok := byDir[dir]; ok {
			existing.Local = existing.Local || local
			existing.Saved = existing.Saved || !local

			return
		}

		cached := gui.stacks.get(dir, !local || gui.localStackExplicit, gui.loadStack)
		if local && !gui.localStackExplicit && cached.Err != nil {
			return
		}

		stack := *cached
		stack.Local, stack.Saved = local, !local
		byDir[dir] = &stack
		stacks = append(stacks, &stack)
	}

	if gui.localStackDir != "" {
		add(gui.localStackDir, true)
	}

	for _, dir := range saved {
		add(dir, false)
	}

	return stacks
}

// fetchStacks reads the stacks' configs, cached, and every compose
// instance's status: one listing, whatever the number of stacks.
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

	statuses, err := gui.IncusCommand.GetComposeStatuses()
	if err != nil {
		return nil, err
	}

	for _, stack := range stacks {
		if stack.Name != "" {
			stack.Statuses = statuses[stack.Name]
		}
	}

	return func() error {
		if !gui.refreshes.stacks.admit(ticket) {
			return nil
		}

		gui.Panels.Stacks.SetItems(stacks)

		if err := gui.Panels.Stacks.RerenderList(); err != nil {
			return err
		}

		if err := gui.setStackProjects(stacks); err != nil {
			return err
		}

		// nil when there are none.
		selected, _ := gui.Panels.Stacks.GetSelectedItem()

		return gui.followStack(selected)
	}, nil
}

// setStackProjects hands the stacks' compose instances to the services
// panel, re-filtering the instances panel when that changes which.
func (gui *Gui) setStackProjects(stacks []*commands.ComposeStack) error {
	projects := map[string]bool{}

	for _, stack := range stacks {
		if stack.Name != "" {
			projects[stack.Name] = true
		}
	}

	if maps.Equal(projects, gui.State.StackProjects) {
		return nil
	}

	gui.State.StackProjects = projects
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
// directory, and the project its compose file names.
func stackIdentity(stack *commands.ComposeStack) string {
	if stack == nil {
		return ""
	}

	return stack.Dir + "\x00" + stack.Name
}

// renderStackLogs stacks every service's logs, each instance under a
// heading of its own, the way a replicated service's Logs tab stacks its
// replicas'.
func (gui *Gui) renderStackLogs(stack *commands.ComposeStack) tasks.TaskFunc {
	return gui.renderLogsToMain(func() string { return gui.stackLogsStr(stack) })
}

func (gui *Gui) stackLogsStr(stack *commands.ComposeStack) string {
	if stack.Err != nil {
		return utils.ColoredString(stack.Err.Error(), color.FgRed)
	}

	state := gui.composeInstances.Load()
	if state == nil || state.project != stack.Name {
		return ""
	}

	var sections []string

	for _, service := range sortedServices(state.services) {
		for _, instance := range service.SortedInstances() {
			heading := service.Name
			if instance.Name != service.Name {
				heading += " · " + instance.Name
			}

			sections = append(sections, gui.sectionHeading(heading)+"\n\n"+gui.instanceLogStr(instance))
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

	return gui.switchFocus(gui.Views.Main)
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

// handleStackAdd is `a`: a directory to list, saved in state.yml.
func (gui *Gui) handleStackAdd(g *gocui.Gui, v *gocui.View) error {
	return gui.openTextPrompt(gui.Tr.AddStackPrompt, gui.Tr.AddStackHint, func(input string) error {
		return gui.WithWaitingStatus(gui.Tr.AddingStackStatus, func() error {
			return gui.addStack(input)
		})
	})
}

// addStack takes a directory only once incus-compose has read a compose
// project from it: a stack that's never going to work is a row of noise.
// Off the main loop.
func (gui *Gui) addStack(input string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	dir, err := commands.ResolveStackDir(input, cwd, gui.home)
	if err != nil {
		return err
	}

	if err := commands.CheckStackDir(dir); err != nil {
		return err
	}

	state, err := gui.Config.LoadAppState()
	if err != nil {
		return err
	}

	if slices.Contains(state.Stacks, dir) || gui.isLocalStack(dir) {
		return fmt.Errorf(gui.Tr.StackAlreadyListed, dir)
	}

	stack := gui.loadStack(dir)
	if stack.Err != nil {
		return fmt.Errorf(gui.Tr.StackNotComposeProject, dir, stack.Err)
	}

	if err := gui.Config.AddStack(dir); err != nil {
		if errors.Is(err, config.ErrStackListed) {
			return fmt.Errorf(gui.Tr.StackAlreadyListed, dir)
		}

		return err
	}

	gui.stacks.put(stack)

	return gui.refresh(func() error { return gui.selectStack(dir) }, gui.fetchStacks)
}

// isLocalStack is whether dir is the local stack's, listed.
func (gui *Gui) isLocalStack(dir string) bool {
	if dir != gui.localStackDir {
		return false
	}

	return gui.localStackExplicit || gui.stacks.get(dir, false, gui.loadStack).Err == nil
}

// selectStack moves the Stacks panel's cursor to dir's row, and the focus
// to the panel.
func (gui *Gui) selectStack(dir string) error {
	index := gui.Panels.Stacks.List.GetIndexBy(func(stack *commands.ComposeStack) bool { return stack.Dir == dir })
	if index < 0 {
		return nil
	}

	gui.Panels.Stacks.SetSelectedLineIdx(index)

	if err := gui.switchFocus(gui.Views.Stacks); err != nil {
		return err
	}

	return gui.Panels.Stacks.RerenderList()
}

// stackRemove is `D`: forget a saved stack, after asking. Nothing on the
// daemon changes, and the local stack isn't saved to be removed.
func (gui *Gui) stackRemove(stack *commands.ComposeStack) error {
	path := commands.ShortenHome(stack.Dir, gui.home)

	if !stack.Saved {
		return gui.createErrorPanel(fmt.Sprintf(gui.Tr.CannotRemoveLocalStack, path))
	}

	message := fmt.Sprintf(gui.Tr.ConfirmRemoveStack, stack.Title(), path)

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := gui.Config.RemoveStack(stack.Dir); err != nil {
				return err
			}

			return gui.refresh(nil, gui.fetchStacks)
		})
	}, nil)
}
