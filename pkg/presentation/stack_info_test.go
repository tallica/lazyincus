package presentation

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func composeInstance(name, service string, config map[string]string) *commands.Instance {
	expanded := map[string]string{"user.label.incus-compose.service": service}
	for key, value := range config {
		expanded[key] = value
	}

	return &commands.Instance{Name: name, Instance: api.InstanceFull{Instance: api.Instance{
		Name: name, Status: "Running", ExpandedConfig: expanded,
	}}}
}

// Drift is an orphan the file has dropped, a declared service with nothing
// behind it, and instances created from an older config; a service that
// matches has none.
func TestStackDrift(t *testing.T) {
	old := composeInstance("api", "api", map[string]string{"user.label.incus-compose.config-hash": "v1:old"})

	state := &StackInstances{
		Services: []*commands.ComposeService{
			{Name: "web", Instances: []*commands.Instance{composeInstance("web", "web", nil)}},
			{Name: "cache"},
			{Name: "api", Hash: "v1:new", Instances: []*commands.Instance{old}},
		},
		Orphans: []*commands.Instance{composeInstance("worker-1", "worker", nil)},
	}

	assert.Equal(t,
		"api:          1 instance from an older config, recreate with u\n"+
			"cache:        declared, not created\n"+
			"worker:       1 instance, running, not in the compose file\n",
		StackDrift(state))
}
