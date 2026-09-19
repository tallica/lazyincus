package gui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) getServicesPanel() *panels.SideListPanel[*commands.ComposeService] {
	return &panels.SideListPanel[*commands.ComposeService]{
		ContextState: &panels.ContextState[*commands.ComposeService]{
			GetMainTabs: func() []panels.MainTab[*commands.ComposeService] {
				return []panels.MainTab[*commands.ComposeService]{
					{
						Key:    "info",
						Title:  gui.Tr.InfoTitle,
						Render: gui.renderServiceInfo,
					},
					{
						Key:    "logs",
						Title:  gui.Tr.LogsTitle,
						Render: gui.renderServiceLogs,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderServiceConfig,
					},
				}
			},
			GetItemContextCacheKey: func(service *commands.ComposeService) string {
				return "services-" + service.Name + "-" + service.Status()
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeService]{
			List: panels.NewFilteredList[*commands.ComposeService](),
			View: gui.Views.Services,
		},
		NoItemsMessage: gui.Tr.NoServices,
		Gui:            gui.intoInterface(),
		// No compose file in the working directory means no services to act
		// on, and the panel would be a title over an empty list.
		Hide: gui.noLocalComposeProject,
		// Compose file order is arbitrary (a JSON object), so name is the
		// only stable order there is.
		Sort: func(a, b *commands.ComposeService) bool {
			return a.Name < b.Name
		},
		GetTableCells: presentation.GetComposeServiceDisplayStrings,
	}
}

// singleInstance is the instance a per-instance key should act on: the
// service's only one. With replicas there's no single answer, so the caller
// asks which - see withServiceInstance.
func singleInstance(service *commands.ComposeService) (*commands.Instance, bool) {
	if len(service.Instances) == 1 {
		return service.Instances[0], true
	}

	return nil, false
}

func (gui *Gui) renderServiceInfo(service *commands.ComposeService) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.serviceInfoStr(service) })
}

func (gui *Gui) serviceInfoStr(service *commands.ComposeService) string {
	padding := 14

	output := utils.WithPadding("Service: ", padding) + service.Name + "\n"
	output += utils.WithPadding("Project: ", padding) + service.Project + "\n"

	if service.Image != "" {
		output += utils.WithPadding("Image: ", padding) + service.Image + "\n"
	}

	output += utils.WithPadding("Replicas: ", padding) + serviceReplicasStr(service) + "\n"

	if health := service.Health(); health != "" {
		output += utils.WithPadding("Health: ", padding) + health + "\n"
	}

	if project := gui.State.ComposeProject; project != nil {
		output += utils.WithPadding("Healthcheck: ", padding) + gui.composeHealthcheckStr(project) + "\n"
		output += utils.WithPadding("Resources: ", padding) + composeResourceCountsStr(project) + "\n"
	}

	return output + "\n" + gui.Tr.ComposeManageHint + "\n\n" + gui.serviceInstancesStr(service)
}

// serviceReplicasStr shows what's running against what the compose file
// asked for, since the two disagreeing is the reason to look.
func serviceReplicasStr(service *commands.ComposeService) string {
	return fmt.Sprintf("%d/%d", len(service.Instances), service.Replicas)
}

// serviceInstancesStr is the `incus-compose ps` table, narrowed to one
// service - so no SERVICE column, unlike the command's own output.
func (gui *Gui) serviceInstancesStr(service *commands.ComposeService) string {
	if len(service.Instances) == 0 {
		return gui.Tr.ServiceNotRunning
	}

	instances := append([]*commands.Instance(nil), service.Instances...)
	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })

	rows := [][]string{{"INSTANCE", "IMAGE", "STATUS", "ADDRESSES"}}
	for _, instance := range instances {
		rows = append(rows, []string{
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

// renderServiceLogs delegates to the instance logs renderer, which owns the
// drain-on-read console buffer. Merging several replicas' buffers into one
// ordered stream is a different problem - see BACKLOG.md's aggregate-logs
// item - so a replicated service points at `incus-compose logs` instead.
func (gui *Gui) renderServiceLogs(service *commands.ComposeService) tasks.TaskFunc {
	instance, ok := singleInstance(service)
	if !ok {
		return gui.NewSimpleRenderStringTask(func() string {
			if len(service.Instances) == 0 {
				return gui.Tr.ServiceNotRunning
			}

			return gui.Tr.ServiceLogsMultipleInstances
		})
	}

	return gui.renderInstanceLogsToMain(instance)
}

func (gui *Gui) renderServiceConfig(service *commands.ComposeService) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.serviceConfigStr(service) })
}

// serviceConfigStr is the selected service's slice of `incus-compose
// config`, re-rendered as YAML: the command prints the whole project, and
// the panel is already one row per service.
func (gui *Gui) serviceConfigStr(service *commands.ComposeService) string {
	cmd := gui.OSCommand.NewCmd("incus-compose", "config", "--format", "json")

	output, err := gui.OSCommand.RunExecutableWithOutput(cmd)
	if err != nil {
		return fmt.Sprintf("Error running `incus-compose config`: %v", err)
	}

	// The service's JSON is handed to YAML untouched: decoding it through
	// map[string]any first turns every count in the compose file into a
	// float64, and `replicas: 2` renders as 2.0.
	var config struct {
		Services map[string]json.RawMessage `json:"services"`
	}

	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return err.Error()
	}

	definition, ok := config.Services[service.Name]
	if !ok {
		return gui.Tr.ServiceNotInComposeFile
	}

	data, err := yaml.JSONToYAML(definition)
	if err != nil {
		return err.Error()
	}

	return utils.ColoredYamlString(string(data))
}

func (gui *Gui) refreshServices() error {
	if gui.Views.Services == nil || gui.noLocalComposeProject() {
		return nil
	}

	services, err := gui.IncusCommand.GetComposeServices(
		gui.State.LocalComposeProject, gui.State.ComposeServiceDefs)
	if err != nil {
		return err
	}

	// The project's own config backs the Info tab's healthcheck and resource
	// lines; fetched here so rendering stays free of API calls.
	if project, err := gui.IncusCommand.GetComposeProject(gui.State.LocalComposeProject); err != nil {
		gui.Log.Warn(err)
	} else {
		gui.State.ComposeProject = project
	}

	gui.Panels.Services.SetItems(services)

	return gui.Panels.Services.RerenderList()
}

// refreshServicesQuiet is the background poll. A service's instances change
// under a compose verb, and every one of those calls refreshAfterCompose
// already - so this only has to catch changes made from outside lazyincus.
func (gui *Gui) refreshServicesQuiet() error {
	if err := gui.refreshServices(); err != nil {
		gui.Log.Warn(err)
	}

	return nil
}

// composeConfigOutput is the slice of `incus-compose config --format json`
// lazyincus reads: the project name it would act on, and the services it
// declares.
type composeConfigOutput struct {
	Name     string `json:"name"`
	Services map[string]struct {
		Image  string `json:"image"`
		Deploy struct {
			Replicas *int `json:"replicas"`
		} `json:"deploy"`
	} `json:"services"`
}

// parseComposeConfig extracts the project name and the declared services
// from `incus-compose config --format json` output. The name is the one
// incus-compose itself would act on (the directory, unless
// -p/INCUS_COMPOSE_PROJECT_NAME or the file overrides it).
func parseComposeConfig(output string) (string, []commands.ComposeService, error) {
	var cfg composeConfigOutput
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		return "", nil, err
	}

	services := make([]commands.ComposeService, 0, len(cfg.Services))

	for name, definition := range cfg.Services {
		replicas := 1
		if definition.Deploy.Replicas != nil {
			replicas = *definition.Deploy.Replicas
		}

		services = append(services, commands.ComposeService{
			Name:     name,
			Image:    definition.Image,
			Replicas: replicas,
		})
	}

	return cfg.Name, services, nil
}

// localComposeProject shells out to `incus-compose config --format json` to
// find the compose project, if any, whose compose file lives in lazyincus's
// own working directory - the only one the services panel can act on.
// Absence of a compose file here (incus-compose exits 1 with "no
// compose.yaml found") isn't an error worth surfacing: most servers running
// compose stacks aren't being administered from this directory.
//
// It resolves the remote before parsing, so it's a real (if fast)
// subprocess call.
func (gui *Gui) localComposeProject() (string, []commands.ComposeService) {
	cmd := gui.OSCommand.NewCmd("incus-compose", "config", "--format", "json")

	output, err := gui.OSCommand.RunExecutableWithOutput(cmd)
	if err != nil {
		gui.Log.Info(err)
		return "", nil
	}

	name, services, err := parseComposeConfig(output)
	if err != nil {
		gui.Log.Warn(err)
		return "", nil
	}

	return name, services
}

// selectedService is the services panel's selection. No selection is a
// silent no-op, matching the other panel handlers.
func (gui *Gui) selectedService() (*commands.ComposeService, bool) {
	service, err := gui.Panels.Services.GetSelectedItem()
	if err != nil {
		return nil, false
	}

	return service, true
}

// composeRun is every compose verb: the arguments, then the service to
// narrow them to. An empty service means the whole project, which is what
// the verb does with no SERVICE argument.
func (gui *Gui) composeRun(service string, args ...string) error {
	if service != "" {
		args = append(args, service)
	}

	if err := gui.runSubprocess(gui.OSCommand.NewCmd("incus-compose", args...)); err != nil {
		return err
	}

	return gui.refreshAfterCompose()
}

// composeTarget names what a confirmation prompt is about - the service, or
// the whole project when no service narrows it.
func (gui *Gui) composeTarget(service string) string {
	if service != "" {
		return fmt.Sprintf(gui.Tr.ComposeTargetService, service)
	}

	return fmt.Sprintf(gui.Tr.ComposeTargetProject, gui.State.LocalComposeProject)
}

// composeConfirm wraps a verb in a confirmation naming its target, for the
// ones that interrupt something running or destroy it.
func (gui *Gui) composeConfirm(prompt, service string, args ...string) error {
	message := fmt.Sprintf(prompt, gui.composeTarget(service))

	return gui.createConfirmationPanel(gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		return gui.composeRun(service, args...)
	}, nil)
}

func (gui *Gui) handleComposeUp(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	// --detach: without it, `up` stays attached tailing every service's logs,
	// which would leave the keypress looking hung until the user Ctrl-C's it.
	return gui.composeRun(service.Name, "up", "--detach")
}

// handleComposeUpPullRecreate is `U`: `up --pull always --recreate`,
// replacing an instance already running an older image.
func (gui *Gui) handleComposeUpPullRecreate(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeConfirm(gui.Tr.ConfirmComposeUpPullRecreate, service.Name,
		"up", "--pull", "always", "--recreate", "--detach")
}

// handleComposeStart runs `incus-compose start` - already-created instances
// only, unlike `u`, which also creates whatever's missing.
func (gui *Gui) handleComposeStart(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "start")
}

func (gui *Gui) handleComposeStop(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeConfirm(gui.Tr.ConfirmComposeStop, service.Name, "stop")
}

func (gui *Gui) handleComposeRestart(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "restart")
}

func (gui *Gui) handleComposeDown(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeDownMenu(service.Name)
}

func (gui *Gui) composeDownMenu(service string) error {
	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ComposeDownMenuTitle,
		Items: []*types.MenuItem{
			{
				Label: gui.Tr.ComposeDownOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDown, service, "down")
				},
			},
			{
				Label: gui.Tr.ComposeDownWithVolumesOption,
				OnPress: func() error {
					return gui.composeConfirm(gui.Tr.ConfirmComposeDownWithVolumes, service, "down", "--volumes")
				},
			},
		},
	})
}

// handleComposeMenu is `C`: the verbs that don't warrant a key of their own,
// each listed twice - once narrowed to the selected service, once for the
// whole project, which is the same command with the SERVICE argument left
// off.
func (gui *Gui) handleComposeMenu(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	verbs := []struct {
		label   string
		args    []string
		confirm string
	}{
		{gui.Tr.ComposeKill, []string{"kill"}, gui.Tr.ConfirmComposeKill},
		{gui.Tr.ComposePause, []string{"pause"}, ""},
		{gui.Tr.ComposeUnpause, []string{"unpause"}, ""},
		{gui.Tr.ComposeBuild, []string{"build"}, ""},
		{gui.Tr.ComposePull, []string{"pull"}, ""},
		{gui.Tr.ComposeLogs, []string{"logs", "--follow"}, ""},
	}

	items := make([]*types.MenuItem, 0, len(verbs)*2)

	for _, target := range []string{service.Name, ""} {
		for _, verb := range verbs {
			label := fmt.Sprintf("%s %s", verb.label, gui.composeTarget(target))

			items = append(items, &types.MenuItem{
				Label:   label,
				OnPress: gui.composeMenuAction(target, verb.confirm, verb.args),
			})
		}
	}

	return gui.Menu(CreateMenuOptions{Title: gui.Tr.ComposeMenuTitle, Items: items})
}

func (gui *Gui) composeMenuAction(service, confirm string, args []string) func() error {
	return func() error {
		if confirm != "" {
			return gui.composeConfirm(confirm, service, args...)
		}

		return gui.composeRun(service, args...)
	}
}

// withServiceInstance runs a per-instance action against the service's
// instance. A service usually has exactly one, and then the key acts
// straight away; replicas have no single answer, so it asks which.
func (gui *Gui) withServiceInstance(title string, action func(*commands.Instance) error) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	if len(service.Instances) == 0 {
		return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
	}

	if instance, ok := singleInstance(service); ok {
		return action(instance)
	}

	instances := append([]*commands.Instance(nil), service.Instances...)
	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })

	items := make([]*types.MenuItem, 0, len(instances))

	for _, instance := range instances {
		items = append(items, &types.MenuItem{
			Label:   instance.Name,
			OnPress: func() error { return action(instance) },
		})
	}

	return gui.Menu(CreateMenuOptions{Title: title, Items: items})
}

func (gui *Gui) handleServiceExecShell(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.ExecShell, gui.instanceExecShell)
}

func (gui *Gui) handleServiceAttach(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.Attach, gui.instanceAttachConsole)
}

func (gui *Gui) handleServiceCopyIPv4(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.CopyIPv4, gui.instanceCopyIPv4)
}

func (gui *Gui) handleServiceSnapshotCreate(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.NewSnapshot, gui.snapshotCreatePrompt)
}

func (gui *Gui) handleServiceViewLogs(g *gocui.Gui, v *gocui.View) error {
	if err := gui.Panels.Services.SetMainTab("logs"); err != nil {
		return err
	}

	return gui.switchFocus(gui.Views.Main)
}

// refreshAfterCompose re-lists the services and the instances panel once a
// compose verb returns, rather than waiting on the next background poll to
// notice what it created or removed.
func (gui *Gui) refreshAfterCompose() error {
	if err := gui.refreshInstances(); err != nil {
		return err
	}

	return gui.refreshServices()
}
