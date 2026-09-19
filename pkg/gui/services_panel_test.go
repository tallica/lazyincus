package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func TestParseComposeConfig(t *testing.T) {
	name, services, err := parseComposeConfig(`{
		"name": "playground",
		"services": {
			"web": {"image": "images:alpine/3.20"},
			"db": {"image": "postgres:16", "deploy": {"replicas": 2}}
		}
	}`)

	assert.NoError(t, err)
	assert.Equal(t, "playground", name)

	byName := map[string]commands.ComposeService{}
	for _, service := range services {
		byName[service.Name] = service
	}

	assert.Equal(t, "images:alpine/3.20", byName["web"].Image)
	// Replicas defaults to one, the way compose reads a service with no
	// deploy block.
	assert.Equal(t, 1, byName["web"].Replicas)
	assert.Equal(t, 2, byName["db"].Replicas)
}

// The long forms compose normalizes every short one to, as
// `incus-compose config --format json` prints them.
func TestParseComposeConfigServiceDefinition(t *testing.T) {
	_, services, err := parseComposeConfig(`{
		"name": "zigbee2mqtt",
		"services": {
			"zigbee2mqtt": {
				"image": "koenkk/zigbee2mqtt:2.14",
				"command": ["mosquitto", "-c", "/mosquitto-no-auth.conf"],
				"restart": "unless-stopped",
				"depends_on": {"mosquitto": {"condition": "service_started"}},
				"ports": [
					{"target": 8080, "published": "8191", "protocol": "tcp"},
					{"target": 5353, "published": "5353", "protocol": "udp"}
				],
				"volumes": [
					{"type": "volume", "source": "zigbee2mqtt_data", "target": "/app/data"},
					{"type": "bind", "source": "/run/udev", "target": "/run/udev", "read_only": true}
				],
				"devices": [{"source": "/dev/ttyUSB0", "target": "/dev/ttyUSB0"}]
			}
		}
	}`)

	assert.NoError(t, err)
	assert.Len(t, services, 1)

	service := services[0]
	assert.Equal(t, "mosquitto -c /mosquitto-no-auth.conf", service.Command)
	assert.Equal(t, "unless-stopped", service.Restart)
	assert.Equal(t, []string{"8191:8080", "5353:5353/udp"}, service.Ports)
	assert.Equal(t, []string{"zigbee2mqtt_data:/app/data", "/run/udev:/run/udev (ro)"}, service.Volumes)
	assert.Equal(t, []string{"/dev/ttyUSB0:/dev/ttyUSB0"}, service.Devices)
	assert.Equal(t, []string{"mosquitto"}, service.DependsOn)
}

// A compose file compose couldn't normalize leaves a string command
// through, which mustn't fail the parse and empty the panel.
func TestParseComposeConfigStringCommand(t *testing.T) {
	_, services, err := parseComposeConfig(`{
		"name": "playground",
		"services": {"web": {"command": "nginx -g daemon off;"}}
	}`)

	assert.NoError(t, err)
	assert.Len(t, services, 1)
	assert.Equal(t, "nginx -g daemon off;", services[0].Command)
}

func TestParseComposeConfigNoServices(t *testing.T) {
	name, services, err := parseComposeConfig(`{"name": "playground"}`)

	assert.NoError(t, err)
	assert.Equal(t, "playground", name)
	assert.Empty(t, services)
}

func TestParseComposeConfigInvalidJSON(t *testing.T) {
	_, _, err := parseComposeConfig("not json")
	assert.Error(t, err)
}
