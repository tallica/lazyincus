package app

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/log"
)

// App struct
type App struct {
	Config       *config.AppConfig
	Log          *logrus.Entry
	OSCommand    *commands.OSCommand
	IncusCommand *commands.IncusCommand
	Gui          *gui.Gui
	Tr           *i18n.TranslationSet
}

// NewApp bootstraps a new application
func NewApp(config *config.AppConfig) (*App, error) {
	app := &App{Config: config}
	var err error
	app.Log = log.NewLogger(config)
	app.Tr, err = i18n.NewTranslationSetFromConfig(app.Log, config.UserConfig.Gui.Language)
	if err != nil {
		return app, err
	}
	app.OSCommand = commands.NewOSCommand(app.Log, config)

	app.IncusCommand, err = commands.NewIncusCommand(app.Log, app.OSCommand, app.Tr, app.Config)
	if err != nil {
		return app, err
	}
	app.Gui, err = gui.NewGui(app.Log, app.IncusCommand, app.OSCommand, app.Tr, config)
	if err != nil {
		return app, err
	}
	return app, nil
}

func (app *App) Run() error {
	return app.Gui.Run()
}

type errorMapping struct {
	originalError string
	newError      string
}

// KnownError takes an error and tells us whether it's an error that we know
// about where we can print a nicely formatted version of it rather than
// panicking with a stack trace
func (app *App) KnownError(err error) (string, bool) {
	errorMessage := err.Error()

	// A socket we're not allowed to open is the one connection failure with
	// advice of its own, so the mappings come before the general case.
	mappings := []errorMapping{
		{
			originalError: "permission denied",
			newError:      app.Tr.CannotAccessIncusSocketError,
		},
	}

	for _, mapping := range mappings {
		if strings.Contains(errorMessage, mapping.originalError) {
			return mapping.newError, true
		}
	}

	// A stack trace says nothing about an unreachable daemon that the
	// address doesn't.
	if commands.IsConnectError(err) {
		return fmt.Sprintf(app.Tr.CannotReachDaemonError, err), true
	}

	return "", false
}
