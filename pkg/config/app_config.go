// Package config handles all the user-configuration. The fields here are
// all in PascalCase but in your actual config.yml they'll be in camelCase.
// You can view the default config with `lazyincus --config`.
// You can open your config file with 'o', or edit it in $VISUAL/$EDITOR
// with 'O'. Changes are picked up without a restart, aside from the few
// options noted in docs/Config.md as startup-only.
package config

import (
	"os"
	"path/filepath"

	"github.com/OpenPeeDeeP/xdg"
	"github.com/jesseduffield/yaml"
)

// UserConfig holds all of the user-configurable options
type UserConfig struct {
	// Gui is for configuring visual things like colors and whether we show or
	// hide things
	Gui GuiConfig `yaml:"gui,omitempty"`

	// ConfirmOnQuit when enabled prompts you to confirm you want to quit when you
	// hit esc or q when no confirmation panels are open
	ConfirmOnQuit bool `yaml:"confirmOnQuit,omitempty"`

	// OS determines what defaults are set for opening files and links
	OS OSConfig `yaml:"oS,omitempty"`

	// For demo purposes: any list item with one of these strings as a substring
	// will be filtered out and not displayed.
	Ignore []string `yaml:"ignore,omitempty"`
}

// ThemeConfig is for setting the colors of panels and some text.
type ThemeConfig struct {
	ActiveBorderColor   []string `yaml:"activeBorderColor,omitempty"`
	InactiveBorderColor []string `yaml:"inactiveBorderColor,omitempty"`
	SelectedLineBgColor []string `yaml:"selectedLineBgColor,omitempty"`
	OptionsTextColor    []string `yaml:"optionsTextColor,omitempty"`
}

// GuiConfig is for configuring visual things like colors and whether we show or
// hide things
type GuiConfig struct {
	// ScrollHeight determines how many characters you scroll at a time when
	// scrolling the main panel
	ScrollHeight int `yaml:"scrollHeight,omitempty"`

	// Language determines which language the GUI is displayed in. English is
	// currently the only supported language.
	Language string `yaml:"language,omitempty"`

	// ScrollPastBottom determines whether you can scroll past the bottom of the
	// main view
	ScrollPastBottom bool `yaml:"scrollPastBottom,omitempty"`

	// IgnoreMouseEvents is for when you do not want to use your mouse to interact
	// with anything
	IgnoreMouseEvents bool `yaml:"mouseEvents,omitempty"`

	// Theme determines what colors and color attributes your panel borders have.
	Theme ThemeConfig `yaml:"theme,omitempty"`

	// ReturnImmediately determines whether you get the 'press enter to return to
	// lazyincus' message after a subprocess has completed.
	ReturnImmediately bool `yaml:"returnImmediately,omitempty"`

	// WrapMainPanel determines whether we use word wrap on the main panel
	WrapMainPanel bool `yaml:"wrapMainPanel,omitempty"`

	// If 0.333, then the side panel will be 1/3 of the screen's width
	SidePanelWidth float64 `yaml:"sidePanelWidth"`

	// ExpandFocusedSidePanel gives the focused side panel the space the
	// others aren't using, collapsing them to their title. Useful once
	// there are enough panels that an even split leaves each one short.
	ExpandFocusedSidePanel bool `yaml:"expandFocusedSidePanel,omitempty"`

	// Determines whether we show the bottom line (the one containing keybinding
	// info and the status of the app).
	ShowBottomLine bool `yaml:"showBottomLine"`

	// ScreenMode allow user to specify which screen mode will be used on startup
	ScreenMode string `yaml:"screenMode,omitempty"`

	// Determines the style of the instance status display in the instances
	// panel. "long": full words (default), "short": one or two characters,
	// "icon": unicode emoji.
	InstanceStatusStyle string `yaml:"instanceStatusStyle"`

	// Window border style.
	// One of 'rounded' (default) | 'single' | 'double' | 'hidden'
	Border string `yaml:"border"`

	// InstanceColumns controls which columns the Instances panel shows, and
	// in what order. Valid values: "name", "status", "type", "ipv4", "ipv6",
	// "project", "service", "snapshots". Unknown values are ignored; omitted
	// values are simply not shown. "project" is added automatically whenever
	// the list spans more than one project.
	InstanceColumns []string `yaml:"instanceColumns,omitempty"`
}

// DefaultInstanceColumns is the Instances panel's column set/order when the
// user hasn't customized InstanceColumns.
var DefaultInstanceColumns = []string{"name", "status", "type", "ipv4", "ipv6", "snapshots"}

// OSConfig contains config on the level of the os
type OSConfig struct {
	// OpenCommand is the command for opening a file
	OpenCommand string `yaml:"openCommand,omitempty"`

	// OpenLinkCommand is the command for opening a link
	OpenLinkCommand string `yaml:"openLinkCommand,omitempty"`

	// CopyToClipboardCommand is the command that text to be copied is piped
	// into (on its stdin). When left empty, lazyincus uses the first of
	// pbcopy, wl-copy, xclip or xsel that it finds on your PATH.
	CopyToClipboardCommand string `yaml:"copyToClipboardCommand,omitempty"`
}

// GetDefaultConfig returns the application default configuration NOTE (to
// contributors, not users): do not default a boolean to true, because false is
// the boolean zero value and this will be ignored when parsing the user's
// config
func GetDefaultConfig() UserConfig {
	return UserConfig{
		Gui: GuiConfig{
			ScrollHeight:      2,
			Language:          "en",
			ScrollPastBottom:  false,
			IgnoreMouseEvents: false,
			Theme: ThemeConfig{
				ActiveBorderColor:   []string{"green", "bold"},
				InactiveBorderColor: []string{"default"},
				SelectedLineBgColor: []string{"blue"},
				OptionsTextColor:    []string{"blue"},
			},
			ReturnImmediately:      false,
			ExpandFocusedSidePanel: false,
			WrapMainPanel:          true,
			SidePanelWidth:         0.3333,
			ShowBottomLine:         true,
			ScreenMode:             "normal",
			InstanceStatusStyle:    "long",
			InstanceColumns:        DefaultInstanceColumns,
		},
		ConfirmOnQuit: false,
		OS:            GetPlatformDefaultConfig(),
	}
}

// AppConfig contains the base configuration fields required for lazyincus.
type AppConfig struct {
	Debug       bool   `long:"debug" env:"DEBUG" default:"false"`
	Version     string `long:"version" env:"VERSION" default:"unversioned"`
	Commit      string `long:"commit" env:"COMMIT"`
	BuildDate   string `long:"build-date" env:"BUILD_DATE"`
	Name        string `long:"name" env:"NAME" default:"lazyincus"`
	BuildSource string `long:"build-source" env:"BUILD_SOURCE" default:""`
	UserConfig  *UserConfig
	ConfigDir   string
}

// NewAppConfig makes a new app config
func NewAppConfig(name, version, commit, date string, buildSource string, debuggingFlag bool) (*AppConfig, error) {
	configDir, err := findOrCreateConfigDir(name)
	if err != nil {
		return nil, err
	}

	userConfig, err := loadUserConfigWithDefaults(configDir)
	if err != nil {
		return nil, err
	}

	appConfig := &AppConfig{
		Name:        name,
		Version:     version,
		Commit:      commit,
		BuildDate:   date,
		Debug:       debuggingFlag || os.Getenv("DEBUG") == "TRUE",
		BuildSource: buildSource,
		UserConfig:  userConfig,
		ConfigDir:   configDir,
	}

	return appConfig, nil
}

// configDirForVendor checks CONFIG_DIR, XDG_CONFIG_HOME, an existing
// ~/.config/<projectName>, then the platform default. The third case is why
// macOS users with a config synced from Linux are picked up without moving
// it: xdg.New would only ever look at ~/Library/Application Support.
func configDirForVendor(vendor string, projectName string) string {
	envConfigDir := os.Getenv("CONFIG_DIR")
	if envConfigDir != "" {
		return envConfigDir
	}

	if os.Getenv("XDG_CONFIG_HOME") != "" {
		return xdg.New(vendor, projectName).ConfigHome()
	}

	xdgDefault := filepath.Join(os.Getenv("HOME"), ".config", vendor, projectName)
	if _, err := os.Stat(xdgDefault); err == nil {
		return xdgDefault
	}

	return xdg.New(vendor, projectName).ConfigHome()
}

func configDir(projectName string) string {
	return configDirForVendor("", projectName)
}

func findOrCreateConfigDir(projectName string) (string, error) {
	folder := configDir(projectName)

	err := os.MkdirAll(folder, 0o755)
	if err != nil {
		return "", err
	}

	return folder, nil
}

func loadUserConfigWithDefaults(configDir string) (*UserConfig, error) {
	config := GetDefaultConfig()

	return loadUserConfig(configDir, &config)
}

func loadUserConfig(configDir string, base *UserConfig) (*UserConfig, error) {
	fileName := filepath.Join(configDir, "config.yml")

	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			file, err := os.Create(fileName)
			if err != nil {
				return nil, err
			}
			file.Close()
		} else {
			return nil, err
		}
	}

	content, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, base); err != nil {
		return nil, err
	}

	return base, nil
}

// WriteToUserConfig allows you to set a value on the user config to be saved
// note that if you set a zero-value, it may be ignored e.g. a false or 0 or
// empty string this is because we are using the omitempty yaml directive so
// that we don't write a heap of zero values to the user's config.yml
func (c *AppConfig) WriteToUserConfig(updateConfig func(*UserConfig) error) error {
	userConfig, err := loadUserConfig(c.ConfigDir, &UserConfig{})
	if err != nil {
		return err
	}

	if err := updateConfig(userConfig); err != nil {
		return err
	}

	file, err := os.OpenFile(c.ConfigFilename(), os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		return err
	}

	return yaml.NewEncoder(file).Encode(userConfig)
}

// ReloadUserConfig re-reads the config file, layered over the defaults again
// so that a removed key reverts to its default.
func (c *AppConfig) ReloadUserConfig() error {
	userConfig, err := loadUserConfigWithDefaults(c.ConfigDir)
	if err != nil {
		return err
	}

	c.UserConfig = userConfig

	return nil
}

// ConfigFilename returns the filename of the current config file
func (c *AppConfig) ConfigFilename() string {
	return filepath.Join(c.ConfigDir, "config.yml")
}
