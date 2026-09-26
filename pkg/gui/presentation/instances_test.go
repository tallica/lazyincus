package presentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestShortImageRef(t *testing.T) {
	tests := []struct {
		ref      string
		expected string
	}{
		{"docker.io/library/nginx:alpine", "nginx:alpine"},
		{"ghcr.io/lxc/incus-compose/ic-healthd:latest", "ic-healthd:latest"},
		{"docker.io/koenkk/zigbee2mqtt:latest", "zigbee2mqtt:latest"},
		{"localhost:5000/myapp:dev", "myapp:dev"},
		{"alpine/3.20", "alpine/3.20"},
		{"images:debian/12", "images:debian/12"},
		{"ubuntu", "ubuntu"},
		{"", ""},
	}

	for _, test := range tests {
		if actual := shortImageRef(test.ref); actual != test.expected {
			t.Errorf("shortImageRef(%q) = %q, want %q", test.ref, actual, test.expected)
		}
	}
}

// The image column is found where the config put it, counting only the
// columns that render: a project column ahead of it, and an unknown name
// that shows nothing, both move it.
func TestImageColumn(t *testing.T) {
	assert.Equal(t, -1, InstanceImageColumn(&config.GuiConfig{}, false))

	columns := &config.GuiConfig{InstanceColumns: []string{"name", "nonsense", "image", "status"}}
	assert.Equal(t, 1, InstanceImageColumn(columns, false))
	assert.Equal(t, 2, InstanceImageColumn(columns, true))

	assert.Equal(t, -1, ServiceImageColumn(&config.GuiConfig{}))
	assert.Equal(t, 2, ServiceImageColumn(&config.GuiConfig{ServiceColumns: []string{"name", "status", "image"}}))
}
