package commands

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// composeConfig runs `incus-compose config --format json` for the compose
// file in the working directory. It resolves the remote first, so it's a
// real, if fast, subprocess call.
func (c *IncusCommand) composeConfig() (string, error) {
	return c.OSCommand.RunExecutableWithOutput(c.OSCommand.NewCmd("incus-compose", "config", "--format", "json"))
}

// LocalComposeConfig is the compose project in the working directory: the
// name incus-compose would act on and the services its file declares.
func (c *IncusCommand) LocalComposeConfig() (string, []ComposeService, error) {
	output, err := c.composeConfig()
	if err != nil {
		return "", nil, err
	}

	return parseComposeConfig(output)
}

// ComposeServiceConfig is one service's definition from the compose file,
// as incus-compose normalized it, in YAML. found is false for a service the
// file no longer declares.
func (c *IncusCommand) ComposeServiceConfig(service string) (definition string, found bool, err error) {
	output, err := c.composeConfig()
	if err != nil {
		return "", false, err
	}

	// The service's JSON is handed to YAML untouched: decoding it through
	// map[string]any first turns every count in the compose file into a
	// float64, and `replicas: 2` renders as 2.0.
	var config struct {
		Services map[string]json.RawMessage `json:"services"`
	}

	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return "", false, err
	}

	raw, ok := config.Services[service]
	if !ok {
		return "", false, nil
	}

	data, err := yaml.JSONToYAML(raw)
	if err != nil {
		return "", false, err
	}

	return string(data), true, nil
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
func parseComposeConfig(output string) (string, []ComposeService, error) {
	var cfg composeConfigOutput
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		return "", nil, err
	}

	services := make([]ComposeService, 0, len(cfg.Services))

	for name, definition := range cfg.Services {
		replicas := 1
		if definition.Deploy.Replicas != nil {
			replicas = *definition.Deploy.Replicas
		}

		services = append(services, ComposeService{
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
