package presentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestGetComposeServiceDisplayStrings(t *testing.T) {
	service := &commands.ComposeService{Name: "web", Image: "docker.io/library/nginx:alpine", Replicas: 2}

	assert.Equal(t,
		[]string{"web", "none", "", "", "0"},
		GetComposeServiceDisplayStrings(&config.GuiConfig{}, service))

	// The replica count is blank when it matches what was declared, and the
	// status follows gui.instanceStatusStyle like an instance's.
	matched := &commands.ComposeService{Name: "web", Replicas: 0}
	assert.Equal(t,
		[]string{"-", ""},
		GetComposeServiceDisplayStrings(
			&config.GuiConfig{
				InstanceStatusStyle: "short",
				ServiceColumns:      []string{"status", "replicas"},
			}, matched))

	assert.Equal(t,
		[]string{"0/2", "web"},
		GetComposeServiceDisplayStrings(
			&config.GuiConfig{ServiceColumns: []string{"replicas", "nonsense", "name"}}, service))
}
