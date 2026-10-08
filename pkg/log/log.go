package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/rs/zerolog"
	"github.com/tallica/lazyincus/pkg/config"
)

// Formats are the values --log-format takes, the first the default.
var Formats = []string{"console", "json"}

// ValidFormat reports whether format is one of Formats.
func ValidFormat(format string) bool {
	return slices.Contains(Formats, format)
}

// NewLogger returns a new logger
func NewLogger(config *config.AppConfig) *zerolog.Logger {
	if !config.Debug && os.Getenv("DEBUG") != "TRUE" {
		log := zerolog.Nop()
		return &log
	}

	file, err := os.OpenFile(filepath.Join(config.ConfigDir, "development.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Println("unable to log to file")
		os.Exit(1)
	}

	log := zerolog.New(writer(file, config.LogFormat)).Level(getLogLevel()).With().Timestamp().Logger()
	log.Info().
		Str("version", config.Version).
		Str("commit", config.Commit).
		Str("buildDate", config.BuildDate).
		Msg("starting")

	return &log
}

func writer(out io.Writer, format string) io.Writer {
	if format == "json" {
		return out
	}

	return zerolog.ConsoleWriter{Out: out, NoColor: true, TimeFormat: time.RFC3339}
}

func getLogLevel() zerolog.Level {
	// ParseLevel takes "" for NoLevel, which would log everything.
	level, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || level == zerolog.NoLevel {
		return zerolog.DebugLevel
	}
	return level
}
