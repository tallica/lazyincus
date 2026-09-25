package commands

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/lxc/incus/v7/shared/api"
)

// A compose instance's lifecycle, done the way incus-compose does it to each
// of its own instances. Its CLI takes whole services only - given an
// instance name, `stop` quietly does nothing - so a replica's row can't
// hand one replica to it. What this follows is incus-compose's documented
// behaviour, not its internals:
//
//   - user.healthcheck.stopped is ic-healthd's documented input ("Config
//     Storage" in its docs/root/healthd.md): true says a stop or pause was
//     deliberate, so a restart policy leaves the instance alone.
//   - stop is docker's: a clean shutdown, then a kill once the timeout is
//     up. Incus fails that shutdown and leaves the instance running.
//   - restart, pause and unpause follow its
//     docs/root/cli-reference/lifecycle.md ("Pausing and health checks").
//
// Should incus-compose come to take one instance, these go to its CLI.

const healthStoppedKey = "user.healthcheck.stopped"

// The timeouts incus-compose's stop and restart default to, so a replica
// gets as long here as its service would there.
const (
	composeStopTimeout    = 10
	composeRestartTimeout = 60
)

func (i *Instance) isCompose() bool {
	return i.ComposeService() != ""
}

func (i *Instance) composeStart() error {
	if err := i.markStopped(false); err != nil {
		return err
	}

	return i.updateState("start", -1, false)
}

func (i *Instance) composeStop(timeout int) error {
	return i.whileMarkedStopped(func() error {
		err := i.updateState("stop", timeout, false)
		if err == nil {
			return nil
		}

		state, _, stateErr := i.Client.GetInstanceState(i.Name)
		if stateErr != nil {
			return errors.Join(err, stateErr)
		}

		// Only Stopped is done: Incus refuses to shut down an instance in
		// Error cleanly, and a stop that failed mid-freeze or mid-stop
		// leaves one of those states behind. A forced stop takes any of them.
		if state.StatusCode == api.Stopped {
			return nil
		}

		return i.updateState("stop", -1, true)
	})
}

func (i *Instance) composeRestart() error {
	if err := i.composeStop(composeRestartTimeout); err != nil {
		return err
	}

	return i.composeStart()
}

// whileMarkedStopped takes the mark off again should action fail: left on a
// replica still running, it would keep ic-healthd from restarting one that
// later crashed. incus-compose itself leaves it on.
func (i *Instance) whileMarkedStopped(action func() error) error {
	if err := i.markStopped(true); err != nil {
		return err
	}

	err := action()
	if err == nil {
		return nil
	}

	return errors.Join(err, i.markStopped(false))
}

func (i *Instance) composeUnfreeze() error {
	if err := i.updateState("unfreeze", -1, false); err != nil {
		return err
	}

	return i.markStopped(false)
}

// markStopped writes healthStoppedKey with a PATCH of that one key, as
// incus-compose writes it: ic-healthd stamps its verdict into the same
// config on a timer, and a read-modify-write would race it. The client has
// no PATCH of its own, and its raw query adds neither the API version nor
// the project.
func (i *Instance) markStopped(stopped bool) error {
	path := "/1.0/instances/" + url.PathEscape(i.Name) + "?" + url.Values{"project": {i.Project}}.Encode()
	patch := map[string]any{"config": map[string]string{healthStoppedKey: strconv.FormatBool(stopped)}}

	return i.retryWhileBusy(func() error {
		_, _, err := i.Client.RawQuery("PATCH", path, patch, "")

		return err
	})
}
