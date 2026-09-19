package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
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
	running := &Instance{Instance: api.Instance{Status: "Running"}}
	stopped := &Instance{Instance: api.Instance{Status: "Stopped"}}
	frozen := &Instance{Instance: api.Instance{Status: "Frozen"}}

	tests := []struct {
		name      string
		instances []*Instance
		want      string
	}{
		{"nothing running", nil, ServiceNone},
		{"all running", []*Instance{running, running}, ServiceRunning},
		{"all stopped", []*Instance{stopped}, ServiceStopped},
		{"one of two", []*Instance{running, stopped}, ServicePartial},
		{"frozen counts as not running", []*Instance{running, frozen}, ServicePartial},
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
		instance := &Instance{}
		instance.setFull(&api.InstanceFull{
			Instance: api.Instance{
				ExpandedConfig: map[string]string{healthStatusKey: status},
			},
		})

		return instance
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
