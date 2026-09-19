package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/go-errors/errors"
	"github.com/integrii/flaggy"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/app"
	"github.com/tallica/lazyincus/pkg/config"
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
)

// composeProjectDirectory resolves the --project-directory flag to an
// absolute path, and refuses one that isn't a directory: incus-compose
// answers a bad path by reporting no compose project at all, which
// lazyincus reads as "no stack here" and shows as a missing Services panel
// - a typo would look like the feature not working.
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

	flaggy.Bool(&debuggingFlag, "d", "debug", "a boolean")
	flaggy.String(&remoteFlag, "r", "remote", "Incus remote to talk to, overriding INCUS_REMOTE and the CLI's default-remote")
	flaggy.String(&projectDirectory, "P", "project-directory", "Directory to look for the compose file in, overriding INCUS_COMPOSE_PROJECT_DIRECTORY")
	flaggy.SetVersion(info)

	flaggy.Parse()

	// Both flags are applied as the environment variables they name rather
	// than threaded inward, because the client is only half of the app:
	// `incus console`, `incus exec` and every incus-compose verb are
	// subprocesses that resolve the remote and the compose file themselves.
	// Setting the variables here puts the panels and every shell-out on the
	// same daemon and the same stack, which passing values inward wouldn't.
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

	lazyincusApp, err := app.NewApp(appConfig)
	if err == nil {
		err = lazyincusApp.Run()
	}
	lazyincusApp.Close()

	if err != nil {
		if errMessage, known := lazyincusApp.KnownError(err); known {
			log.Println(errMessage)
			os.Exit(0)
		}

		newErr := errors.Wrap(err, 0)
		stackTrace := newErr.ErrorStack()
		lazyincusApp.Log.Error(stackTrace)

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
