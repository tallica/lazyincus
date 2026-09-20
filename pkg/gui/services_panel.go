package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
					{
						Key:    "env",
						Title:  gui.Tr.EnvTitle,
						Render: gui.serviceInstanceTab(gui.renderInstanceEnv),
					},
					{
						Key:    "top",
						Title:  gui.Tr.TopTitle,
						Render: gui.serviceInstanceTab(gui.renderInstanceTopToMain),
					},
				}
			},
			GetItemContextCacheKey: func(service *commands.ComposeService) string {
				// Each refresh builds new ComposeService values, so the
				// instance count is part of the key: without it a replica
				// coming or going leaves the main panel rendering the
				// service object the tab opened with.
				return "services-" + service.Name + "-" + service.Status() +
					"-" + strconv.Itoa(len(service.Instances))
			},
		},
		ListPanel: panels.ListPanel[*commands.ComposeService]{
			List: panels.NewFilteredList[*commands.ComposeService](),
			View: gui.Views.Services,
		},
		NoItemsMessage: gui.Tr.NoServices,
		Gui:            gui.intoInterface(),
		// The snapshots panel shows the selected service's instance while
		// this panel has focus, the way it follows the instances panel. A
		// replicated service points at none: showing one replica's
		// snapshots under the service's name would be a lie.
		OnSelect: func(service *commands.ComposeService) error {
			instance, _ := singleInstance(service)

			return gui.refreshSnapshotsFor(instance)
		},
		// No compose file in the working directory means no services to act
		// on, and the panel would be a title over an empty list.
		Hide: gui.noLocalComposeProject,
		// Compose file order is arbitrary (a JSON object), so name is the
		// only stable order there is.
		Sort: func(a, b *commands.ComposeService) bool {
			return a.Name < b.Name
		},
		GetTableCells: func(service *commands.ComposeService) []string {
			return presentation.GetComposeServiceDisplayStrings(&gui.Config.UserConfig.Gui, service)
		},
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

// renderServiceInfo ticks like the instances panel's Stats tab, the two
// being one tab here: the identity half is static, the counters underneath
// it aren't.
func (gui *Gui) renderServiceInfo(service *commands.ComposeService) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderStringMain(gui.serviceInfoStr(service))
		},
		Duration:   time.Second,
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) serviceInfoStr(service *commands.ComposeService) string {
	padding := 14

	line := func(label, value string) string {
		if value == "" {
			return ""
		}

		return utils.WithPadding(label+": ", padding) + value + "\n"
	}

	output := line("Service", service.Name)
	output += line("Image", service.ResolvedImage())
	output += line("Replicas", presentation.ServiceReplicas(service))
	output += line("Health", service.Health())

	if project := gui.State.ComposeProject; project != nil {
		output += line("Healthcheck", gui.composeHealthcheckStr(project))
	}

	output += line("Command", service.Command)
	output += line("Restart", service.Restart)
	output += line("Ports", strings.Join(service.Ports, ", "))
	output += line("Volumes", strings.Join(service.Volumes, ", "))
	output += line("Devices", strings.Join(service.Devices, ", "))
	output += line("Depends on", strings.Join(service.DependsOn, ", "))

	if len(service.Instances) == 0 {
		return output + "\n" + gui.Tr.ServiceNotRunning
	}

	// Each instance's own Info tab under that, minus the lines the service
	// has just shown: project and image are the same for every replica, and
	// a lone instance's health is what the service's rolls up to.
	omit := []string{"Project", "Image"}
	if _, single := singleInstance(service); single {
		omit = append(omit, "Health")
	}

	for _, instance := range sortedInstances(service) {
		instanceOmit := omit
		// A compose file with a container_name gets an instance named after
		// the service, and the Service line above has already said it.
		if instance.Name == service.Name {
			instanceOmit = append(instanceOmit, "Name")
		}

		output += "\n" + gui.instanceInfoStr(instance, instanceOmit...)
	}

	return output
}

// sortedInstances orders a service's replicas by name, the order they're
// listed in being otherwise whatever the daemon returned.
func sortedInstances(service *commands.ComposeService) []*commands.Instance {
	instances := append([]*commands.Instance(nil), service.Instances...)
	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })

	return instances
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

// serviceInstanceTab is a tab that is really the instance's: the service's
// only one answers for it, and replicas have no single answer - the same
// split the per-instance keys make through withServiceInstance.
func (gui *Gui) serviceInstanceTab(
	render func(*commands.Instance) tasks.TaskFunc,
) func(*commands.ComposeService) tasks.TaskFunc {
	return func(service *commands.ComposeService) tasks.TaskFunc {
		instance, ok := singleInstance(service)
		if !ok {
			return gui.NewSimpleRenderStringTask(func() string {
				return gui.serviceNoSingleInstanceStr(service)
			})
		}

		return render(instance)
	}
}

// serviceNoSingleInstanceStr says which of the two reasons there's no
// instance to show: nothing running, or too many to pick from.
func (gui *Gui) serviceNoSingleInstanceStr(service *commands.ComposeService) string {
	if len(service.Instances) == 0 {
		return gui.Tr.ServiceNotRunning
	}

	return gui.Tr.ServiceMultipleInstances
}

// renderServiceLogs delegates to the instance logs renderer, which owns the
// drain-on-read console buffer. Merging several replicas' buffers into one
// ordered stream is a different problem - see BACKLOG.md's aggregate-logs
// item - so a replicated service points at `C`'s `logs --follow` instead.
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

// serviceConfigStr is both halves of a service's configuration: what the
// compose file declares, then what the daemon made of it.
func (gui *Gui) serviceConfigStr(service *commands.ComposeService) string {
	output := sectionHeading(gui.Tr.ComposeTitle) + "\n\n" + gui.composeServiceConfigStr(service)

	// Then what the daemon made of it: the same dump the instances panel's
	// Config tab shows, one per replica. The heading says what the section
	// is rather than just naming it - a lone instance usually carries the
	// service's own name, and "mosquitto" under "Compose" reads as another
	// view of the compose file.
	_, single := singleInstance(service)

	for _, instance := range sortedInstances(service) {
		heading := gui.Tr.InstanceTitle
		if !single {
			heading += " (" + instance.Name + ")"
		}

		output += "\n\n" + sectionHeading(heading) + "\n\n" + gui.instanceConfigStr(instance)
	}

	return output
}

// composeServiceConfigStr is the selected service's slice of `incus-compose
// config`, re-rendered as YAML: the command prints the whole project, and
// the panel is already one row per service.
func (gui *Gui) composeServiceConfigStr(service *commands.ComposeService) string {
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
	Name     string                       `json:"name"`
	Services map[string]composeServiceDef `json:"services"`
}

// composeServiceDef is one service as `incus-compose config` prints it.
// Compose normalizes every short form to these long ones, so a port is
// always an object and depends_on always a map, whatever the file said.
type composeServiceDef struct {
	Image string `json:"image"`
	// Command is a list once normalized, but a file compose can't normalize
	// leaves a string through; RawMessage so neither shape fails the parse
	// and empties the panel.
	Command json.RawMessage `json:"command"`
	Restart string          `json:"restart"`
	Deploy  struct {
		Replicas *int `json:"replicas"`
	} `json:"deploy"`
	Ports []struct {
		Target    int    `json:"target"`
		Published string `json:"published"`
		Protocol  string `json:"protocol"`
	} `json:"ports"`
	Volumes []struct {
		Source   string `json:"source"`
		Target   string `json:"target"`
		ReadOnly bool   `json:"read_only"`
	} `json:"volumes"`
	Devices []struct {
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"devices"`
	DependsOn map[string]struct{} `json:"depends_on"`
}

// commandStr is the command as a shell line, from either shape.
func (def composeServiceDef) commandStr() string {
	if len(def.Command) == 0 {
		return ""
	}

	var list []string
	if err := json.Unmarshal(def.Command, &list); err == nil {
		return strings.Join(list, " ")
	}

	var line string
	if err := json.Unmarshal(def.Command, &line); err == nil {
		return line
	}

	return ""
}

// portsStr renders published:target, the way `docker compose ps` prints a
// mapping, with the protocol only when it isn't the tcp default.
func (def composeServiceDef) portsStr() []string {
	ports := make([]string, 0, len(def.Ports))

	for _, port := range def.Ports {
		mapping := fmt.Sprintf("%s:%d", port.Published, port.Target)
		if port.Protocol != "" && port.Protocol != "tcp" {
			mapping += "/" + port.Protocol
		}

		ports = append(ports, mapping)
	}

	return ports
}

// volumesStr renders source:target - a volume name or a host path on the
// left, whichever the compose file used.
func (def composeServiceDef) volumesStr() []string {
	volumes := make([]string, 0, len(def.Volumes))

	for _, volume := range def.Volumes {
		mount := volume.Source + ":" + volume.Target
		if volume.ReadOnly {
			mount += " (ro)"
		}

		volumes = append(volumes, mount)
	}

	return volumes
}

func (def composeServiceDef) devicesStr() []string {
	devices := make([]string, 0, len(def.Devices))

	for _, device := range def.Devices {
		devices = append(devices, device.Source+":"+device.Target)
	}

	return devices
}

// dependsOnStr is sorted: the map the JSON decodes to has no order of its
// own, and a list that reshuffles between refreshes reads as a change.
func (def composeServiceDef) dependsOnStr() []string {
	names := make([]string, 0, len(def.DependsOn))

	for name := range def.DependsOn {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
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
			Name:      name,
			Image:     definition.Image,
			Replicas:  replicas,
			Command:   definition.commandStr(),
			Restart:   definition.Restart,
			Ports:     definition.portsStr(),
			Volumes:   definition.volumesStr(),
			Devices:   definition.devicesStr(),
			DependsOn: definition.dependsOnStr(),
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

func (gui *Gui) handleComposeKill(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeConfirm(gui.Tr.ConfirmComposeKill, service.Name, "kill")
}

// handleComposePause is `p`, the toggle the instances panel's own `p` is.
func (gui *Gui) handleComposePause(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	if len(service.Instances) == 0 {
		return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
	}

	return gui.composeRun(service.Name, composePauseVerb(service.Status()))
}

// composePauseVerb is which half of the toggle to run, over one service's
// rolled-up status or every service's at once: frozen throughout thaws,
// anything else freezes. Services with nothing running don't vote.
func composePauseVerb(statuses ...string) string {
	frozen := false

	for _, status := range statuses {
		if status == commands.ServiceNone {
			continue
		}

		if !strings.EqualFold(status, "Frozen") {
			return "pause"
		}

		frozen = true
	}

	if frozen {
		return "unpause"
	}

	return "pause"
}

func (gui *Gui) handleComposeBuild(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "build")
}

func (gui *Gui) handleComposePull(g *gocui.Gui, v *gocui.View) error {
	service, ok := gui.selectedService()
	if !ok {
		return nil
	}

	return gui.composeRun(service.Name, "pull")
}

// handleComposeProjectMenu is `C`: the same verbs the service keys run, with
// the SERVICE argument left off so they act on the whole stack. It takes no
// selection - a project whose services have never been deployed is brought
// up from here.
func (gui *Gui) handleComposeProjectMenu(g *gocui.Gui, v *gocui.View) error {
	// One pause row rather than two, the way `p` is one key: the verb is the
	// stack's own status, every service voting.
	services := gui.Panels.Services.List.GetAllItems()
	statuses := make([]string, 0, len(services))

	for _, service := range services {
		statuses = append(statuses, service.Status())
	}

	pauseVerb := composePauseVerb(statuses...)

	pauseLabel := gui.Tr.ComposePause
	if pauseVerb == "unpause" {
		pauseLabel = gui.Tr.ComposeUnpause
	}

	item := func(label, confirm string, args ...string) *types.MenuItem {
		return &types.MenuItem{Label: label, OnPress: gui.composeMenuAction("", confirm, args)}
	}

	items := []*types.MenuItem{
		item(gui.Tr.ComposeUp, "", "up", "--detach"),
		// Down is the service key's own submenu, `--volumes` and all, with the
		// project as its target.
		{Label: gui.Tr.ComposeDown, OnPress: func() error { return gui.composeDownMenu("") }},
		item(gui.Tr.ComposeUpPullRecreate, gui.Tr.ConfirmComposeUpPullRecreate,
			"up", "--pull", "always", "--recreate", "--detach"),
		item(gui.Tr.Start, "", "start"),
		item(gui.Tr.Stop, gui.Tr.ConfirmComposeStop, "stop"),
		item(gui.Tr.Restart, "", "restart"),
		item(pauseLabel, "", pauseVerb),
		item(gui.Tr.ComposeKill, gui.Tr.ConfirmComposeKill, "kill"),
		item(gui.Tr.ComposeBuild, "", "build"),
		item(gui.Tr.ComposePull, "", "pull"),
		item(gui.Tr.ComposeLogs, "", "logs", "--follow"),
	}

	return gui.Menu(CreateMenuOptions{
		Title: fmt.Sprintf(gui.Tr.ComposeProjectMenuTitle, gui.State.LocalComposeProject),
		Items: items,
	})
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
