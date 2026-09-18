package presentation

import "testing"

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
