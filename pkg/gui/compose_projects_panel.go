package gui

import (
	"encoding/json"
	"fmt"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getComposeProjectsPanel() *panels.SideListPanel[*commands.ComposeProject] {
	return &panels.SideListPanel[*commands.ComposeProject]{
		ContextState: &panels.ContextState[*commands.ComposeProject]{
			GetMainTabs: func() []panels.MainTab[*commands.ComposeProject] {
				return []panels.MainTab[*commands.ComposeProject]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderComposeProjectInfo,
					},
				}
			},
			GetItemContextCacheKey: func(project *commands.ComposeProject) string {
				return "composeProjects-" + project.Name
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeProject]{
			List: panels.NewFilteredList[*commands.ComposeProject](),
			View: gui.Views.ComposeProjects,
		},
		NoItemsMessage: gui.Tr.NoComposeProjects,
		Gui:            gui.intoInterface(),
		Sort: func(a, b *commands.ComposeProject) bool {
			return a.Name < b.Name
		},
		GetTableCells: presentation.GetComposeProjectDisplayStrings,
	}
}

func (gui *Gui) renderComposeProjectInfo(project *commands.ComposeProject) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.composeProjectInfoStr(project) })
}

func (gui *Gui) composeProjectInfoStr(project *commands.ComposeProject) string {
	padding := 8
	output := utils.WithPadding("Name: ", padding) + project.Name + "\n"

	if project.Local {
		output += utils.WithPadding("Local: ", padding) + gui.Tr.Yes + "\n\n" + gui.Tr.ComposeManageHint
	} else {
		output += utils.WithPadding("Local: ", padding) + gui.Tr.No + "\n\n" + gui.Tr.ComposeNotLocalHint
	}

	return output
}

func (gui *Gui) refreshComposeProjects() error {
	if gui.Views.ComposeProjects == nil {
		return nil
	}

	projects, err := gui.IncusCommand.GetComposeProjects()
	if err != nil {
		return err
	}

	for _, project := range projects {
		project.Local = project.Name == gui.State.LocalComposeProject
	}

	gui.Panels.ComposeProjects.SetItems(projects)

	return gui.Panels.ComposeProjects.RerenderList()
}

// refreshComposeProjectsQuiet is the background poll. Compose projects only
// change when someone runs `incus-compose up`/`down` for a new stack, so it
// runs on the same slow cadence as images/volumes/networks.
func (gui *Gui) refreshComposeProjectsQuiet() error {
	if err := gui.refreshComposeProjects(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

type composeConfigOutput struct {
	Name string `json:"name"`
}

// parseComposeProjectName extracts the project name from `incus-compose
// config --format json` output - the same name incus-compose itself would
// act on (the directory, unless -p/INCUS_COMPOSE_PROJECT_NAME or the file
// overrides it).
func parseComposeProjectName(output string) (string, error) {
	var cfg composeConfigOutput
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		return "", err
	}

	return cfg.Name, nil
}

// localComposeProjectName shells out to `incus-compose config --format
// json` to find the compose project, if any, whose compose file lives in
// lazyincus's own working directory - the only one u/d may act on. Absence
// of a compose file here (incus-compose exits 1 with "no compose.yaml
// found") isn't an error worth surfacing: most servers running compose
// stacks aren't being administered from this directory. It resolves the
// remote before parsing, so it's a real (if fast) subprocess call - callers
// run it off the main goroutine.
func (gui *Gui) localComposeProjectName() string {
	cmd := gui.OSCommand.NewCmd("incus-compose", "config", "--format", "json")

	output, err := gui.OSCommand.RunExecutableWithOutput(cmd)
	if err != nil {
		gui.Log.Info(err)
		return ""
	}

	name, err := parseComposeProjectName(output)
	if err != nil {
		gui.Log.Warn(err)
		return ""
	}

	return name
}

func (gui *Gui) handleComposeUp(g *gocui.Gui, v *gocui.View) error {
	project, err := gui.Panels.ComposeProjects.GetSelectedItem()
	if err != nil {
		return nil
	}

	if !project.Local {
		return gui.createErrorPanel(gui.Tr.ComposeCannotManageNonLocal)
	}

	// --detach: without it, `up` stays attached tailing every service's logs,
	// which would leave the keypress looking hung until the user Ctrl-C's it.
	if err := gui.runSubprocess(gui.OSCommand.NewCmd("incus-compose", "up", "--detach")); err != nil {
		return err
	}

	return gui.refreshAfterCompose()
}

func (gui *Gui) handleComposeDown(g *gocui.Gui, v *gocui.View) error {
	project, err := gui.Panels.ComposeProjects.GetSelectedItem()
	if err != nil {
		return nil
	}

	if !project.Local {
		return gui.createErrorPanel(gui.Tr.ComposeCannotManageNonLocal)
	}

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ComposeDownMenuTitle,
		Items: []*types.MenuItem{
			{
				Label:   gui.Tr.ComposeDownOption,
				OnPress: func() error { return gui.confirmComposeDown(project, false) },
			},
			{
				Label:   gui.Tr.ComposeDownWithVolumesOption,
				OnPress: func() error { return gui.confirmComposeDown(project, true) },
			},
		},
	})
}

func (gui *Gui) confirmComposeDown(project *commands.ComposeProject, withVolumes bool) error {
	prompt := fmt.Sprintf(gui.Tr.ConfirmComposeDown, project.Name)
	args := []string{"down"}

	if withVolumes {
		prompt = fmt.Sprintf(gui.Tr.ConfirmComposeDownWithVolumes, project.Name)
		args = append(args, "--volumes")
	}

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		if err := gui.runSubprocess(gui.OSCommand.NewCmd("incus-compose", args...)); err != nil {
			return err
		}

		return gui.refreshAfterCompose()
	}, nil)
}

// refreshAfterCompose re-lists instances and compose projects once
// `incus-compose up`/`down` returns, rather than waiting on the next
// background poll to notice what it created or removed.
func (gui *Gui) refreshAfterCompose() error {
	if err := gui.refreshInstances(); err != nil {
		return err
	}

	return gui.refreshComposeProjects()
}
