package presentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

func TestGetServiceRowDisplayStrings(t *testing.T) {
	service := &commands.ComposeService{Name: "web", Image: "docker.io/library/nginx:alpine", Replicas: 2}
	row := &commands.ServiceRow{Service: service}

	assert.Equal(t,
		[]string{"web", "none", "", "0/2", "", "", "0"},
		GetServiceRowDisplayStrings(&config.GuiConfig{}, row))

	// The replica count is blank when it matches what was declared, and the
	// status follows gui.instanceStatusStyle like an instance's.
	matched := &commands.ServiceRow{Service: &commands.ComposeService{Name: "web", Replicas: 0}}
	assert.Equal(t,
		[]string{"-", ""},
		GetServiceRowDisplayStrings(
			&config.GuiConfig{
				InstanceStatusStyle: "short",
				ServiceColumns:      []string{"status", "replicas"},
			}, matched))

	assert.Equal(t,
		[]string{"0/2", "web"},
		GetServiceRowDisplayStrings(
			&config.GuiConfig{ServiceColumns: []string{"replicas", "nonsense", "name"}}, row))
}

// The replica count is what exists against what was declared, so a service
// whose instances are all stopped still reads as complete.
func TestServiceReplicas(t *testing.T) {
	stopped := func(name string) *commands.Instance { return &commands.Instance{Name: name} }

	assert.Equal(t, "",
		ServiceReplicas(&commands.ComposeService{
			Replicas:  2,
			Instances: []*commands.Instance{stopped("web-1"), stopped("web-2")},
		}))

	assert.Equal(t, "1/2",
		ServiceReplicas(&commands.ComposeService{
			Replicas:  2,
			Instances: []*commands.Instance{stopped("web-1")},
		}))
}

// A replica's row renders the instance columns under the same names, with
// its name indented under the service's and nothing in the column only a
// service has.
func TestGetServiceRowDisplayStringsReplica(t *testing.T) {
	instance := &commands.Instance{Name: "web-2"}
	instance.Instance.Status = "Running"

	row := &commands.ServiceRow{
		Service:  &commands.ComposeService{Name: "web", Replicas: 2, Instances: []*commands.Instance{instance}},
		Instance: instance,
	}

	assert.Equal(t,
		[]string{"  web-2", "running", ""},
		GetServiceRowDisplayStrings(
			&config.GuiConfig{ServiceColumns: []string{"name", "status", "replicas"}}, row))
}

// A service is changed while any instance is, and each replica row says
// whether it is itself.
func TestGetServiceRowDisplayStringsChanged(t *testing.T) {
	instance := func(name, hash string) *commands.Instance {
		instance := &commands.Instance{Name: name}
		instance.Instance.ExpandedConfig = map[string]string{"user.label.incus-compose.config-hash": hash}

		return instance
	}

	current, old := instance("web-1", "v1:new"), instance("web-2", "v1:old")
	service := &commands.ComposeService{Name: "web", Hash: "v1:new", Replicas: 2, Instances: []*commands.Instance{current, old}}
	columns := &config.GuiConfig{ServiceColumns: []string{"name", "changed"}}

	assert.Equal(t, []string{"web", "changed"}, GetServiceRowDisplayStrings(columns, &commands.ServiceRow{Service: service}))
	assert.Equal(t, []string{"  web-1", ""}, GetServiceRowDisplayStrings(columns, &commands.ServiceRow{Service: service, Instance: current}))
	assert.Equal(t, []string{"  web-2", "changed"}, GetServiceRowDisplayStrings(columns, &commands.ServiceRow{Service: service, Instance: old}))

	icons := &config.GuiConfig{InstanceStatusStyle: "icon", ServiceColumns: []string{"changed"}}
	assert.Equal(t, []string{"↻"}, GetServiceRowDisplayStrings(icons, &commands.ServiceRow{Service: service}))
}
