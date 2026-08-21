package commands

import (
	"io"

	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// This file exports dummy constructors for use by tests in other packages

// NewDummyOSCommand creates a new dummy OSCommand for testing
func NewDummyOSCommand() *OSCommand {
	return NewOSCommand(NewDummyLog(), NewDummyAppConfig())
}

// NewDummyAppConfig creates a new dummy AppConfig for testing
func NewDummyAppConfig() *config.AppConfig {
	appConfig := &config.AppConfig{
		Name:        "lazyincus",
		Version:     "unversioned",
		Commit:      "",
		BuildDate:   "",
		Debug:       false,
		BuildSource: "",
		UserConfig:  &config.UserConfig{},
	}
	return appConfig
}

// NewDummyLog creates a new dummy Log for testing
func NewDummyLog() *logrus.Entry {
	log := logrus.New()
	log.Out = io.Discard
	return log.WithField("test", "test")
}

// NewDummyIncusCommand creates a new dummy IncusCommand for testing. It does
// not connect to a real daemon.
func NewDummyIncusCommand() *IncusCommand {
	return NewDummyIncusCommandWithOSCommand(NewDummyOSCommand())
}

// NewDummyIncusCommandWithOSCommand creates a new dummy IncusCommand for testing
func NewDummyIncusCommandWithOSCommand(osCommand *OSCommand) *IncusCommand {
	newAppConfig := NewDummyAppConfig()
	return &IncusCommand{
		Log:       NewDummyLog(),
		OSCommand: osCommand,
		Tr:        i18n.NewTranslationSet(NewDummyLog(), "en"),
		Config:    newAppConfig,
	}
}
