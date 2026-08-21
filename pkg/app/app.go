package app

import (
	"io"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/gui"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/log"
	"github.com/tallica/lazyincus/pkg/utils"
)

// App struct
type App struct {
	closers []io.Closer

	Config       *config.AppConfig
	Log          *logrus.Entry
	OSCommand    *commands.OSCommand
	IncusCommand *commands.IncusCommand
	Gui          *gui.Gui
	Tr           *i18n.TranslationSet
	ErrorChan    chan error
}

// NewApp bootstraps a new application
func NewApp(config *config.AppConfig) (*App, error) {
	app := &App{
		closers:   []io.Closer{},
		Config:    config,
		ErrorChan: make(chan error),
	}
	var err error
	app.Log = log.NewLogger(config, "")
	app.Tr, err = i18n.NewTranslationSetFromConfig(app.Log, config.UserConfig.Gui.Language)
	if err != nil {
		return app, err
	}
	app.OSCommand = commands.NewOSCommand(app.Log, config)

	app.IncusCommand, err = commands.NewIncusCommand(app.Log, app.OSCommand, app.Tr, app.Config, app.ErrorChan)
	if err != nil {
		return app, err
	}
	app.closers = append(app.closers, app.IncusCommand)
	app.Gui, err = gui.NewGui(app.Log, app.IncusCommand, app.OSCommand, app.Tr, config, app.ErrorChan)
	if err != nil {
		return app, err
	}
	return app, nil
}

func (app *App) Run() error {
	return app.Gui.Run()
}

func (app *App) Close() error {
	return utils.CloseMany(app.closers)
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

	mappings := []errorMapping{
		{
			originalError: "permission denied",
			newError:      app.Tr.CannotAccessIncusSocketError,
		},
		{
			originalError: "no such file or directory",
			newError:      app.Tr.CannotAccessIncusSocketError,
		},
	}

	for _, mapping := range mappings {
		if strings.Contains(errorMessage, mapping.originalError) {
			return mapping.newError, true
		}
	}

	return "", false
}
