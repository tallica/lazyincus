package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
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

// getServicesPanel lists a service per row, and - when it has replicas -
// one row per replica under it.
func (gui *Gui) getServicesPanel() *panels.SideListPanel[*commands.ServiceRow] {
	return &panels.SideListPanel[*commands.ServiceRow]{
		ContextState: &panels.ContextState[*commands.ServiceRow]{
			GetMainTabs: func() []panels.MainTab[*commands.ServiceRow] {
				return []panels.MainTab[*commands.ServiceRow]{
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
			GetItemContextCacheKey: func(row *commands.ServiceRow) string {
				// Each refresh builds new ComposeService values, so the
				// instance count is part of the key: without it a replica
				// coming or going leaves the main panel rendering the
				// service object the tab opened with. A replica row adds its
				// own status, the way the instances panel does, so a restart
				// re-reads the log.
				key := "services-" + row.Key() + "-" + row.Service.Status() +
					"-" + strconv.Itoa(len(row.Service.Instances))

				if row.Instance != nil {
					key += "-" + row.Instance.Instance.Status
				}

				return key
			},
		},
		ListPanel: panels.ListPanel[*commands.ServiceRow]{
			List: panels.NewFilteredList[*commands.ServiceRow](),
			View: gui.Views.Services,
		},
		NoItemsMessage: gui.Tr.NoServices,
		Gui:            gui.intoInterface(),
		// The snapshots panel shows the selected row's instances while this
		// panel has focus, the way it follows the instances panel. A
		// service's own row means every replica of it, so the panel holds
		// all of their snapshots, each row naming the replica it came from.
		OnSelect: func(row *commands.ServiceRow) error {
			label := row.Service.Name
			if row.Instance != nil {
				label = row.Instance.Name
			}

			return gui.refreshSnapshotsFor(label, row.Instances()...)
		},
		// No compose file in the working directory means no services to act
		// on, and the panel would be a title over an empty list.
		Hide: gui.noLocalComposeProject,
		// Compose file order is arbitrary (a JSON object), so name is the
		// only stable order there is; a service's replicas follow it, named
		// in order themselves.
		Sort: func(a, b *commands.ServiceRow) bool {
			if a.Service.Name != b.Service.Name {
				return a.Service.Name < b.Service.Name
			}

			if (a.Instance == nil) != (b.Instance == nil) {
				return a.Instance == nil
			}

			return a.Instance != nil && a.Instance.Name < b.Instance.Name
		},
		// Rows are rebuilt on every refresh, so the cursor follows the one
		// it was on by name rather than by pointer.
		SameItem: func(a, b *commands.ServiceRow) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(row *commands.ServiceRow) []string {
			return presentation.GetServiceRowDisplayStrings(&gui.Config.UserConfig.Gui, row)
		},
	}
}

// renderServiceInfo ticks like the instances panel's Stats tab, the two
// being one tab here: the identity half is static, the counters underneath
// it aren't.
func (gui *Gui) renderServiceInfo(row *commands.ServiceRow) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderStringMain(gui.serviceInfoStr(row))
		},
		Duration:   time.Second,
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) serviceInfoStr(row *commands.ServiceRow) string {
	service := row.Service
	// The same column the instance blocks below use: the tab is one run of
	// labels, the service's and then its instances'.
	padding := identityPadding

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

	if project := gui.composeProject.Load(); project != nil {
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

	// Each instance's own Info tab under that - the selected replica's
	// alone, or every one of them from the service's own row - each under a
	// heading of its own, and minus the lines the service or that heading
	// has just said: project and image are the same for every replica, the
	// name is the heading's, and a lone instance's health is what the
	// service's rolls up to.
	omit := []string{"Project", "Image", "Name"}
	if len(service.Instances) == 1 {
		omit = append(omit, "Health")
	}

	for _, instance := range row.Instances() {
		output += "\n" + gui.instanceHeading(service, instance) + "\n"
		output += "\n" + gui.instanceInfoStr(instance.Latest(), omit...)
	}

	return output
}

// instanceHeading rules off one instance's block and says which of the
// service's instances it is - a name among a dozen identical labels not
// being enough to place you. The number is over the service's instances
// rather than the ones on screen, so a replica's own row still reads "2 of
// 4". A lone instance has no count worth printing, and one carrying the
// service's own name has nothing left to say: the Service line said it.
func (gui *Gui) instanceHeading(service *commands.ComposeService, instance *commands.Instance) string {
	instances := service.SortedInstances()

	if len(instances) < 2 {
		if instance.Name == service.Name {
			return gui.sectionHeading(gui.Tr.InstanceTitle)
		}

		return gui.sectionHeading(fmt.Sprintf(gui.Tr.ServiceInstanceHeading, instance.Name))
	}

	index := slices.IndexFunc(instances, func(other *commands.Instance) bool {
		return other.Name == instance.Name
	})

	return gui.sectionHeading(fmt.Sprintf(
		gui.Tr.ServiceReplicaHeading, index+1, len(instances), instance.Name))
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

// serviceInstanceTab is a tab that is really the instance's: the row's
// replica answers for it, as does a lone service's only instance - the same
// split the per-instance keys make through withServiceInstance.
func (gui *Gui) serviceInstanceTab(
	render func(*commands.Instance) tasks.TaskFunc,
) func(*commands.ServiceRow) tasks.TaskFunc {
	return func(row *commands.ServiceRow) tasks.TaskFunc {
		instance, ok := row.SelectedInstance()
		if !ok {
			return gui.NewSimpleRenderStringTask(func() string {
				return gui.serviceNoSingleInstanceStr(row.Service)
			})
		}

		return render(instance)
	}
}

// serviceNoSingleInstanceStr says which of the two reasons there's no
// instance to show: nothing running, or a row per replica to pick from.
func (gui *Gui) serviceNoSingleInstanceStr(service *commands.ComposeService) string {
	if len(service.Instances) == 0 {
		return gui.Tr.ServiceNotRunning
	}

	return gui.Tr.ServiceMultipleInstances
}

// renderServiceLogs delegates to the instance logs renderer, which owns the
// drain-on-read console buffer. A replicated service's own row has no
// single stream to show, so it stacks every replica's under a heading of
// its own rather than interleaving them - the buffers carry no timestamps
// to merge on. `C`'s `logs --follow` is still the merged view.
func (gui *Gui) renderServiceLogs(row *commands.ServiceRow) tasks.TaskFunc {
	if instance, ok := row.SelectedInstance(); ok {
		return gui.renderInstanceLogsToMain(instance)
	}

	instances := row.Instances()
	if len(instances) == 0 {
		return gui.NewSimpleRenderStringTask(func() string { return gui.Tr.ServiceNotRunning })
	}

	return gui.renderLogsToMain(func() string { return gui.serviceLogsStr(row.Service, instances) })
}

func (gui *Gui) serviceLogsStr(service *commands.ComposeService, instances []*commands.Instance) string {
	sections := make([]string, 0, len(instances))

	for _, instance := range instances {
		sections = append(sections, gui.instanceHeading(service, instance)+"\n\n"+gui.instanceLogStr(instance))
	}

	return strings.Join(sections, "\n\n")
}

func (gui *Gui) renderServiceConfig(row *commands.ServiceRow) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.serviceConfigStr(row) })
}

// serviceConfigStr is both halves of a service's configuration: what the
// compose file declares, then what the daemon made of it.
func (gui *Gui) serviceConfigStr(row *commands.ServiceRow) string {
	service := row.Service
	output := gui.sectionHeading(gui.Tr.ComposeTitle) + "\n\n" + gui.composeServiceConfigStr(service)

	// Then what the daemon made of it: the same dump the instances panel's
	// Config tab shows, one per instance the row stands for, under the Info
	// tab's own headings.
	for _, instance := range row.Instances() {
		output += "\n\n" + gui.instanceHeading(service, instance) +
			"\n\n" + gui.instanceConfigStr(instance)
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

// fetchServices reads LocalComposeProject and ComposeServiceDefs off the
// main loop, which is safe only because both are set once, before it starts.
func (gui *Gui) fetchServices() (func() error, error) {
	if gui.noLocalComposeProject() {
		return func() error { return nil }, nil
	}

	ticket := gui.refreshes.services.issue()

	services, err := gui.IncusCommand.GetComposeServices(
		gui.State.LocalComposeProject, gui.State.ComposeServiceDefs)
	if err != nil {
		return nil, err
	}

	// The project's own config backs the Info tab's healthcheck and resource
	// lines; fetched here so rendering stays free of API calls.
	project, projectErr := gui.IncusCommand.GetComposeProject(gui.State.LocalComposeProject)
	if projectErr != nil {
		gui.Log.Warn(projectErr)
	}

	return func() error {
		if !gui.refreshes.services.admit(ticket) {
			return nil
		}

		// A nil project is an answer - no longer compose-managed - where an
		// error isn't one.
		if projectErr == nil {
			gui.composeProject.Store(project)
		}

		gui.Panels.Services.SetItems(commands.ServiceRows(services))

		if err := gui.Panels.Services.RerenderList(); err != nil {
			return err
		}

		return gui.renderSnapshots()
	}, nil
}

func (gui *Gui) refreshServices() error {
	return gui.refresh(nil, gui.fetchServices)
}

// refreshServicesQuiet is the background poll. A service's instances change
// under a compose verb, and every one of those calls refreshInstancesAndServices
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

// selectedServiceRow is the services panel's selection. No selection is a
// silent no-op, matching the other panel handlers.
func (gui *Gui) selectedServiceRow() (*commands.ServiceRow, bool) {
	row, err := gui.Panels.Services.GetSelectedItem()
	if err != nil {
		return nil, false
	}

	return row, true
}

// selectedService is the service the selection belongs to - a replica's row
// answers with its own, for the verbs that only a service has.
func (gui *Gui) selectedService() (*commands.ComposeService, bool) {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil, false
	}

	return row.Service, true
}

// composeRowDescription is what a key whose two scopes aren't the same verb
// does from the row selected right now - `d` downs a service but deletes a
// replica. The menu rebuilds its bindings each time it opens, so it can say
// which; the panels don't exist yet at the first call, which is startup
// binding the keys rather than anyone reading them.
func (gui *Gui) composeRowDescription(service, replica string) string {
	if gui.Panels.Services == nil {
		return service
	}

	if row, ok := gui.selectedServiceRow(); ok && row.Instance != nil {
		return replica
	}

	return service
}

// serviceScopedDescription is a verb only a service has, described from a
// replica's row: it still runs against the service, and saying so is what
// keeps "bring up" off a row where bringing one replica up isn't a thing.
func (gui *Gui) serviceScopedDescription(description string) string {
	return gui.composeRowDescription(
		description, fmt.Sprintf(gui.Tr.ComposeServiceScoped, description))
}

// onServiceRow splits a key between the compose verb for a whole service
// and the instances panel's own action for one replica. Only a replica's
// row takes the instance side: a service with a single instance is still
// the compose scope, where `u` and `d` create and destroy what the compose
// file declares.
func (gui *Gui) onServiceRow(
	instanceAction func(*commands.Instance) error,
	serviceAction func(*commands.ComposeService) error,
) error {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil
	}

	if row.Instance != nil {
		return instanceAction(row.Instance)
	}

	return serviceAction(row.Service)
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

	gui.refreshInBackground(gui.fetchInstances, gui.fetchServices)

	return nil
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
	return gui.onServiceRow(gui.instanceStart, func(service *commands.ComposeService) error {
		return gui.composeRun(service.Name, "start")
	})
}

func (gui *Gui) handleComposeStop(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceStop, func(service *commands.ComposeService) error {
		return gui.composeConfirm(gui.Tr.ConfirmComposeStop, service.Name, "stop")
	})
}

func (gui *Gui) handleComposeRestart(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceRestart, func(service *commands.ComposeService) error {
		return gui.composeRun(service.Name, "restart")
	})
}

// handleComposeDown is `d`: the service's `down` submenu, or - on a
// replica's row - deleting that instance, which incus-compose creates again
// on the next `up`, the compose file still asking for it.
func (gui *Gui) handleComposeDown(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instanceDelete, func(service *commands.ComposeService) error {
		return gui.composeDownMenu(service.Name)
	})
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
	return gui.onServiceRow(gui.instanceForceStop, func(service *commands.ComposeService) error {
		return gui.composeConfirm(gui.Tr.ConfirmComposeKill, service.Name, "kill")
	})
}

// handleComposePause is `p`, the toggle the instances panel's own `p` is -
// over the whole service, or over the one replica whose row is selected.
func (gui *Gui) handleComposePause(g *gocui.Gui, v *gocui.View) error {
	return gui.onServiceRow(gui.instancePauseFreeze, func(service *commands.ComposeService) error {
		if len(service.Instances) == 0 {
			return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
		}

		return gui.composeRun(service.Name, composePauseVerb(service.Status()))
	})
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
	rows := gui.Panels.Services.List.GetAllItems()
	statuses := make([]string, 0, len(rows))

	for _, row := range rows {
		// One vote per service: a replica's row carries the same service.
		if row.Instance == nil {
			statuses = append(statuses, row.Service.Status())
		}
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

// withServiceInstance runs a per-instance action against the instance the
// selected row stands for, and acts straight away when there is one. A
// service's own row with replicas under it still asks which.
func (gui *Gui) withServiceInstance(title string, action func(*commands.Instance) error) error {
	row, ok := gui.selectedServiceRow()
	if !ok {
		return nil
	}

	if len(row.Service.Instances) == 0 {
		return gui.createErrorPanel(gui.Tr.ServiceNotRunning)
	}

	if instance, ok := row.SelectedInstance(); ok {
		return action(instance)
	}

	items := make([]*types.MenuItem, 0, len(row.Service.Instances))

	for _, instance := range row.Service.SortedInstances() {
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

// refreshInstancesAndServices re-lists both panels as soon as something has
// changed what they hold, rather than waiting on the next background poll
// to notice. Both, because a compose instance has a row in each.
func (gui *Gui) refreshInstancesAndServices() error {
	return gui.refresh(nil, gui.fetchInstances, gui.fetchServices)
}
