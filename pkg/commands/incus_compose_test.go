package commands

import (
	"reflect"
	"testing"
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

func TestComposeProjectResourceCounts(t *testing.T) {
	project := &ComposeProject{
		UsedBy: []string{
			"/1.0/instances/mosquitto?project=zigbee2mqtt",
			"/1.0/instances/zigbee2mqtt?project=zigbee2mqtt",
			"/1.0/images/889cbfb5abaf?project=zigbee2mqtt",
			"/1.0/profiles/default?project=zigbee2mqtt",
			"/1.0/storage-pools/default/volumes/custom/vol-zigbee2mqtt-data?project=zigbee2mqtt",
			"/1.0/storage-pools/default/volumes/custom/vol-mosquitto-data?project=zigbee2mqtt",
			"/1.0/storage-pools/default?project=zigbee2mqtt",
		},
	}

	want := map[string]int{"instances": 2, "images": 1, "profiles": 1, "volumes": 2}
	if got := project.ResourceCounts(); !reflect.DeepEqual(got, want) {
		t.Errorf("ResourceCounts() = %v, want %v", got, want)
	}
}
