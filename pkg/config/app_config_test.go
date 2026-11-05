package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func loadFrom(t *testing.T, content string) (*UserConfig, error) {
	t.Helper()

	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o600))

	return loadUserConfigWithDefaults(dir)
}

func TestLoadUserConfigKeepsDefaults(t *testing.T) {
	defaults := GetDefaultConfig()

	for _, content := range []string{"", "\n", "# nothing set yet\n", "---\n"} {
		loaded, err := loadFrom(t, content)
		assert.NoError(t, err, "%q", content)
		assert.Equal(t, defaults, *loaded, "%q", content)
	}
}

func TestLoadUserConfigOverridesOnlyWhatItSets(t *testing.T) {
	loaded, err := loadFrom(t, "confirmOnQuit: true\ngui:\n  border: single\n  instanceColumns: [name, ipv4]\n  checkImageUpdatesOnStart: true\n")
	assert.NoError(t, err)

	expected := GetDefaultConfig()
	expected.ConfirmOnQuit = true
	expected.Gui.Border = "single"
	expected.Gui.InstanceColumns = []string{"name", "ipv4"}
	expected.Gui.CheckImageUpdatesOnStart = true

	assert.Equal(t, expected, *loaded)
}

func TestLoadUserConfigIgnoresUnknownKeys(t *testing.T) {
	loaded, err := loadFrom(t, "notAnOption: 3\ngui:\n  sidePanelWidth: 0.5\n")
	assert.NoError(t, err)
	assert.InDelta(t, 0.5, loaded.Gui.SidePanelWidth, 0)
}

func TestLoadUserConfigRejectsMalformedYaml(t *testing.T) {
	_, err := loadFrom(t, "gui:\n  border: [unclosed\n")
	assert.Error(t, err)
}

func TestLoadUserConfigCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()

	loaded, err := loadUserConfigWithDefaults(dir)
	assert.NoError(t, err)
	assert.Equal(t, GetDefaultConfig(), *loaded)
	assert.FileExists(t, filepath.Join(dir, "config.yml"))
}

func TestConfigDirPrecedence(t *testing.T) {
	platformDefault := filepath.Join(".config", "lazyincus")
	if runtime.GOOS == "darwin" {
		platformDefault = filepath.Join("Library", "Application Support", "lazyincus")
	}

	for _, tc := range []struct {
		name, configDir, xdgConfigHome string
		dotConfigExists                bool
		want                           string // relative to HOME, unless absolute
	}{
		{name: "CONFIG_DIR", configDir: "/explicit", xdgConfigHome: "/xdg", dotConfigExists: true, want: "/explicit"},
		{name: "XDG_CONFIG_HOME", xdgConfigHome: "/xdg", dotConfigExists: true, want: "/xdg/lazyincus"},
		{name: "existing ~/.config", dotConfigExists: true, want: ".config/lazyincus"},
		{name: "platform default", want: platformDefault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CONFIG_DIR", tc.configDir)
			t.Setenv("XDG_CONFIG_HOME", tc.xdgConfigHome)
			if tc.dotConfigExists {
				assert.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "lazyincus"), 0o755))
			}

			want := tc.want
			if !filepath.IsAbs(want) {
				want = filepath.Join(home, want)
			}
			assert.Equal(t, want, configDir("lazyincus"))
		})
	}
}
