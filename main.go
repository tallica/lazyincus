package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/go-errors/errors"
	"github.com/integrii/flaggy"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/app"
	"github.com/tallica/lazyincus/pkg/config"
	lazylog "github.com/tallica/lazyincus/pkg/log"
	"github.com/tallica/lazyincus/pkg/utils"
)

const DEFAULT_VERSION = "unversioned"

var (
	commit      string
	version     = DEFAULT_VERSION
	date        string
	buildSource = "unknown"

	debuggingFlag    = false
	remoteFlag       = ""
	projectDirectory = ""
	readOnlyFlag     = false
	logFormatFlag    = lazylog.Formats[0]
)

// composeProjectDirectory resolves the --project-directory flag to an
// absolute path, and refuses one that isn't a directory: a typo is better
// caught here than shown as a stack row with an error.
func composeProjectDirectory(dir string) string {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		log.Fatal(err.Error())
	}

	info, err := os.Stat(absolute)
	if err != nil {
		log.Fatalf("--project-directory %s: %v", dir, err)
	}

	if !info.IsDir() {
		log.Fatalf("--project-directory %s: not a directory", dir)
	}

	return absolute
}

func main() {
	updateBuildInfo()

	info := fmt.Sprintf(
		"%s\nDate: %s\nBuildSource: %s\nCommit: %s\nOS: %s\nArch: %s",
		version,
		date,
		buildSource,
		commit,
		runtime.GOOS,
		runtime.GOARCH,
	)

	flaggy.SetName("lazyincus")
	flaggy.SetDescription("The lazier way to manage everything incus")
	flaggy.DefaultParser.AdditionalHelpPrepend = "https://github.com/tallica/lazyincus"

	flaggy.Bool(&debuggingFlag, "d", "debug", "Write a development.log to the config directory")
	flaggy.String(&remoteFlag, "r", "remote", "Incus remote to talk to, overriding INCUS_REMOTE and the CLI's default-remote")
	flaggy.String(&projectDirectory, "P", "project-directory", "Compose project directory to list first in Stacks, in place of the working directory; overrides INCUS_COMPOSE_PROJECT_DIRECTORY")
	flaggy.Bool(&readOnlyFlag, "", "read-only", "Change nothing on any remote: keys that would are refused")
	flaggy.String(&logFormatFlag, "", "log-format", "Format of the --debug log: "+strings.Join(lazylog.Formats, " or "))
	flaggy.SetVersion(info)

	flaggy.Parse()

	if !lazylog.ValidFormat(logFormatFlag) {
		log.Fatalf("--log-format %s: want %s", logFormatFlag, strings.Join(lazylog.Formats, " or "))
	}

	// Both flags are applied as the environment variables they name rather
	// than threaded inward, because the client is only half of the app:
	// `incus console`, `incus exec` and every incus-compose verb are
	// subprocesses that resolve the remote themselves. Setting the variable
	// here puts the panels and every shell-out on the same daemon, which
	// passing a value inward wouldn't; `R` keeps it current after. -P goes
	// the same way, so the local stack is the one incus-compose would pick by
	// itself; each verb names its own stack's directory regardless.
	if remoteFlag != "" {
		if err := os.Setenv("INCUS_REMOTE", remoteFlag); err != nil {
			log.Fatal(err.Error())
		}
	}

	if projectDirectory != "" {
		if err := os.Setenv("INCUS_COMPOSE_PROJECT_DIRECTORY", composeProjectDirectory(projectDirectory)); err != nil {
			log.Fatal(err.Error())
		}
	}

	appConfig, err := config.NewAppConfig("lazyincus", version, commit, date, buildSource, debuggingFlag)
	if err != nil {
		log.Fatal(err.Error())
	}

	appConfig.ReadOnly = readOnlyFlag
	appConfig.LogFormat = logFormatFlag

	lazyincusApp, err := app.NewApp(appConfig)
	if err == nil {
		err = lazyincusApp.Run()
	}

	if err != nil {
		if errMessage, known := lazyincusApp.KnownError(err); known {
			log.Println(errMessage)
			os.Exit(0)
		}

		newErr := errors.Wrap(err, 0)
		stackTrace := newErr.ErrorStack()
		lazyincusApp.Log.Error().Msg(stackTrace)

		log.Fatalf("%s\n\n%s", lazyincusApp.Tr.ErrorOccurred, stackTrace)
	}
}

func updateBuildInfo() {
	if version == DEFAULT_VERSION {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			revision, ok := lo.Find(buildInfo.Settings, func(setting debug.BuildSetting) bool {
				return setting.Key == "vcs.revision"
			})
			if ok {
				commit = revision.Value
				version = utils.SafeTruncate(revision.Value, 7)
			}

			time, ok := lo.Find(buildInfo.Settings, func(setting debug.BuildSetting) bool {
				return setting.Key == "vcs.time"
			})
			if ok {
				date = time.Value
			}
		}
	}
}
