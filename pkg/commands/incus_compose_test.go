package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
)

func TestIsComposeManagedProject(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   bool
	}{
		{"managed", map[string]string{"user.incus-compose.managed": "true"}, true},
		{"cache project", map[string]string{"features.images": "true"}, false},
		{"nil config", nil, false},
		{"false value", map[string]string{"user.incus-compose.managed": "false"}, false},
	}

	for _, tt := range tests {
		if got := isComposeManagedProject(tt.config); got != tt.want {
			t.Errorf("isComposeManagedProject(%v) = %v, want %v", tt.config, got, tt.want)
		}
	}
}

func TestComposeServiceStatus(t *testing.T) {
	running := &Instance{Instance: api.InstanceFull{Instance: api.Instance{Status: "Running"}}}
	stopped := &Instance{Instance: api.InstanceFull{Instance: api.Instance{Status: "Stopped"}}}
	frozen := &Instance{Instance: api.InstanceFull{Instance: api.Instance{Status: "Frozen"}}}

	tests := []struct {
		name      string
		instances []*Instance
		want      string
	}{
		{"nothing running", nil, ServiceNone},
		{"all running", []*Instance{running, running}, "Running"},
		{"all stopped", []*Instance{stopped}, "Stopped"},
		// A service is its instances: frozen stays frozen rather than
		// flattening into stopped.
		{"all frozen", []*Instance{frozen, frozen}, "Frozen"},
		{"one of two", []*Instance{running, stopped}, ServicePartial},
		{"frozen beside running", []*Instance{running, frozen}, ServicePartial},
	}

	for _, tt := range tests {
		service := &ComposeService{Instances: tt.instances}
		if got := service.Status(); got != tt.want {
			t.Errorf("%s: Status() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestComposeServiceHealth(t *testing.T) {
	withHealth := func(status string) *Instance {
		return &Instance{Instance: api.InstanceFull{
			Instance: api.Instance{
				ExpandedConfig: map[string]string{healthStatusKey: status},
			},
		}}
	}

	tests := []struct {
		name      string
		instances []*Instance
		want      string
	}{
		{"unchecked", []*Instance{{}}, ""},
		{"healthy", []*Instance{withHealth(HealthHealthy)}, HealthHealthy},
		{"one unhealthy replica wins", []*Instance{withHealth(HealthHealthy), withHealth(HealthUnhealthy)}, HealthUnhealthy},
		{"starting outranks healthy", []*Instance{withHealth(HealthHealthy), withHealth(HealthStarting)}, HealthStarting},
	}

	for _, tt := range tests {
		service := &ComposeService{Instances: tt.instances}
		if got := service.Health(); got != tt.want {
			t.Errorf("%s: Health() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// A service with replicas lists them under it; one with a single instance
// is that instance, and gets no row of its own.
func TestServiceRows(t *testing.T) {
	replica := func(name string) *Instance { return &Instance{Name: name} }

	web := &ComposeService{
		Name:      "web",
		Instances: []*Instance{replica("web-2"), replica("web-1")},
	}
	redis := &ComposeService{Name: "redis", Instances: []*Instance{replica("redis-1")}}
	down := &ComposeService{Name: "down"}

	rows := ServiceRows([]*ComposeService{web, redis, down})

	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.Key())
	}

	assert.Equal(t, []string{"web", "web/web-1", "web/web-2", "redis", "down"}, keys)

	// The service's own row means all of its replicas; each replica's row
	// means itself, and a lone instance answers for its service.
	_, ok := rows[0].SelectedInstance()
	assert.False(t, ok)
	assert.Len(t, rows[0].Instances(), 2)

	instance, ok := rows[1].SelectedInstance()
	assert.True(t, ok)
	assert.Equal(t, "web-1", instance.Name)

	instance, ok = rows[3].SelectedInstance()
	assert.True(t, ok)
	assert.Equal(t, "redis-1", instance.Name)

	_, ok = rows[4].SelectedInstance()
	assert.False(t, ok)
	assert.Empty(t, rows[4].Instances())
}
