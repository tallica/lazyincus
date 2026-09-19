package gui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderComposeConfig,
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
	padding := 14
	output := utils.WithPadding("Name: ", padding) + project.Name + "\n"

	if project.Description != "" {
		output += utils.WithPadding("Description: ", padding) + project.Description + "\n"
	}

	output += utils.WithPadding("Local: ", padding) + gui.yesNo(project.Local) + "\n"
	output += utils.WithPadding("Healthcheck: ", padding) + gui.composeHealthcheckStr(project) + "\n"
	output += utils.WithPadding("Resources: ", padding) + composeResourceCountsStr(project) + "\n\n"

	if project.Local {
		output += gui.Tr.ComposeManageHint
	} else {
		output += gui.Tr.ComposeNotLocalHint
	}

	return output + "\n\n" + gui.composeProjectServicesStr(project)
}

// composeProjectServicesStr lists the project's instances the way
// `incus-compose ps` does - service, instance, image, status, addresses.
func (gui *Gui) composeProjectServicesStr(project *commands.ComposeProject) string {
	instances, err := gui.IncusCommand.GetProjectInstances(project.Name)
	if err != nil {
		return err.Error()
	}

	if len(instances) == 0 {
		return gui.Tr.NoComposeServices
	}

	sort.Slice(instances, func(i, j int) bool {
		if instances[i].ComposeService() != instances[j].ComposeService() {
			return instances[i].ComposeService() < instances[j].ComposeService()
		}

		return instances[i].Name < instances[j].Name
	})

	rows := [][]string{{"SERVICE", "INSTANCE", "IMAGE", "STATUS", "ADDRESSES"}}
	for _, instance := range instances {
		rows = append(rows, []string{
			instance.ComposeService(),
			instance.Name,
			instance.ComposeImage(),
			strings.ToLower(instance.Instance.Status),
			strings.Join(instance.Addresses("inet"), " "),
		})
	}

	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}

	return table
}

func (gui *Gui) composeHealthcheckStr(project *commands.ComposeProject) string {
	if !project.HealthcheckEnabled() {
		return gui.Tr.No
	}

	if scope := project.HealthcheckScope(); scope != "" {
		return fmt.Sprintf("%s (scope: %s)", gui.Tr.Yes, scope)
	}

	return gui.Tr.Yes
}

// composeResourceKinds orders ResourceCounts for display; a kind absent
// from the project's UsedBy is skipped rather than shown as zero.
var composeResourceKinds = []struct{ key, singular string }{
	{"instances", "instance"},
	{"images", "image"},
	{"volumes", "volume"},
	{"networks", "network"},
	{"profiles", "profile"},
}

func composeResourceCountsStr(project *commands.ComposeProject) string {
	counts := project.ResourceCounts()

	var parts []string

	for _, kind := range composeResourceKinds {
		n, ok := counts[kind.key]
		if !ok {
			continue
		}

		label := kind.singular
		if n != 1 {
			label += "s"
		}

		parts = append(parts, fmt.Sprintf("%d %s", n, label))
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}

func (gui *Gui) renderComposeConfig(project *commands.ComposeProject) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.composeConfigStr(project) })
}

// composeConfigStr renders `incus-compose config`'s plain YAML - not the
// --format json parseComposeProjectName parses - for the local project;
// gated the same way `u`/`d` are, just above.
func (gui *Gui) composeConfigStr(project *commands.ComposeProject) string {
	if !project.Local {
		return gui.Tr.ComposeNotLocalHint
	}

	output, err := gui.OSCommand.RunExecutableWithOutput(gui.OSCommand.NewCmd("incus-compose", "config"))
	if err != nil {
		return fmt.Sprintf("Error running `incus-compose config`: %v", err)
	}

	return utils.ColoredYamlString(output)
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

// handleComposeUpPullRecreate is `U`: `incus-compose up --pull always
// --recreate`, replacing any instance already running an older image.
func (gui *Gui) handleComposeUpPullRecreate(g *gocui.Gui, v *gocui.View) error {
	project, err := gui.Panels.ComposeProjects.GetSelectedItem()
	if err != nil {
		return nil
	}

	if !project.Local {
		return gui.createErrorPanel(gui.Tr.ComposeCannotManageNonLocal)
	}

	prompt := fmt.Sprintf(gui.Tr.ConfirmComposeUpPullRecreate, project.Name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		cmd := gui.OSCommand.NewCmd("incus-compose", "up", "--pull", "always", "--recreate", "--detach")
		if err := gui.runSubprocess(cmd); err != nil {
			return err
		}

		return gui.refreshAfterCompose()
	}, nil)
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
