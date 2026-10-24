package presentation

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

func TestGetServiceRowDisplayStrings(t *testing.T) {
	service := &commands.ComposeService{Name: "web", Image: "docker.io/library/nginx:alpine", Replicas: 2}
	row := &commands.ServiceRow{Service: service}

	assert.Equal(t,
		[]string{"web", "none", "0/2", "", "", "0"},
		GetServiceRowDisplayStrings(&config.GuiConfig{}, row, nil))

	// The replica count is blank when it matches what was declared, and the
	// status follows gui.instanceStatusStyle like an instance's.
	matched := &commands.ServiceRow{Service: &commands.ComposeService{Name: "web", Replicas: 0}}
	assert.Equal(t,
		[]string{"-", ""},
		GetServiceRowDisplayStrings(
			&config.GuiConfig{
				InstanceStatusStyle: "short",
				ServiceColumns:      []string{"status", "replicas"},
			}, matched, nil))

	assert.Equal(t,
		[]string{"0/2", "web"},
		GetServiceRowDisplayStrings(
			&config.GuiConfig{ServiceColumns: []string{"replicas", "nonsense", "name"}}, row, nil))
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
			&config.GuiConfig{ServiceColumns: []string{"name", "status", "replicas"}}, row, nil))
}

// A service whose replica came from an image with a newer build upstream
// says so in its image column, as the replica's own row does.
func TestServiceImageMarker(t *testing.T) {
	instance := &commands.Instance{Name: "web-1"}
	instance.Instance.Config = map[string]string{"volatile.base_image": "old"}

	service := &commands.ComposeService{Name: "web", Image: "docker.io/library/nginx:alpine", Instances: []*commands.Instance{instance}}
	updates := map[string]commands.UpstreamStatus{"old": {State: commands.UpstreamNewer}}
	guiConfig := &config.GuiConfig{ServiceColumns: []string{"name", "image"}}

	plain := func(cells []string) []string {
		return lo.Map(cells, func(cell string, _ int) string { return utils.Decolorise(cell) })
	}

	assert.Equal(t, []string{"web", "nginx:alpine ↑"},
		plain(GetServiceRowDisplayStrings(guiConfig, &commands.ServiceRow{Service: service}, updates)))
	assert.Equal(t, []string{"  web-1", "↑"},
		plain(GetServiceRowDisplayStrings(guiConfig, &commands.ServiceRow{Service: service, Instance: instance}, updates)))

	updates["old"] = commands.UpstreamStatus{State: commands.UpstreamCurrent}
	assert.Equal(t, []string{"web", "nginx:alpine"},
		plain(GetServiceRowDisplayStrings(guiConfig, &commands.ServiceRow{Service: service}, updates)))
}
