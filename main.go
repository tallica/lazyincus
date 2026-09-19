package main

import (
	"fmt"
	"log"
	"os"
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

	debuggingFlag = false
	remoteFlag    = ""
)

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
	flaggy.SetVersion(info)

	flaggy.Parse()

	// The flag is applied as INCUS_REMOTE rather than threaded through to
	// the client, because the client is only half of it: `incus console`,
	// `incus exec` and every incus-compose verb are subprocesses that read
	// the environment themselves. Setting it here puts the panels and every
	// shell-out on the same daemon, which passing a name inward wouldn't.
	if remoteFlag != "" {
		if err := os.Setenv("INCUS_REMOTE", remoteFlag); err != nil {
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
