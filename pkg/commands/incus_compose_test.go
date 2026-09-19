package commands

import "testing"

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
