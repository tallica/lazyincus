package gui

import (
	"testing"

	"github.com/tallica/lazyincus/pkg/config"
)

func TestGetScreenMode(t *testing.T) {
	for _, tc := range []struct {
		configured string
		expected   WindowMaximisation
	}{
		{"normal", SCREEN_NORMAL},
		{"half", SCREEN_HALF},
		{"full", SCREEN_FULL},
		{"fullscreen", SCREEN_FULL},
		{"", SCREEN_NORMAL},
		{"nonsense", SCREEN_NORMAL},
	} {
		cfg := &config.AppConfig{UserConfig: &config.UserConfig{}}
		cfg.UserConfig.Gui.ScreenMode = tc.configured

		if actual := getScreenMode(cfg); actual != tc.expected {
			t.Errorf("screenMode %q: expected %v, got %v", tc.configured, tc.expected, actual)
		}
	}
}
