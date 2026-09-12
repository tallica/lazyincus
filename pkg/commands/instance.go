package commands

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Instance represents an Incus instance (either a container or a VM).
type Instance struct {
	Name string

	// Instance holds the summary data returned by GetInstances (status, type,
	// creation time, ...).
	Instance api.Instance

	Client       incus.InstanceServer
	OSCommand    *OSCommand
	Log          *logrus.Entry
	IncusCommand LimitedIncusCommand
	Tr           *i18n.TranslationSet

	detailsMutex deadlock.Mutex
	// full holds the full instance details (including current state), lazily
	// populated in the background by IncusCommand.RefreshInstanceDetails.
	full *api.InstanceFull

	logMutex          deadlock.Mutex
	logBuffer         strings.Builder
	stoppedLogFetched bool
}

// maxConsoleLogBufferBytes caps how much accumulated console output
// TailConsoleLog keeps per instance, to bound memory for long-running
// sessions.
const maxConsoleLogBufferBytes = 256 * 1024

func (i *Instance) setFull(full *api.InstanceFull) {
	i.detailsMutex.Lock()
	defer i.detailsMutex.Unlock()
	i.full = full
}

// Full returns the last-fetched full instance details, if any.
func (i *Instance) Full() (*api.InstanceFull, bool) {
	i.detailsMutex.Lock()
	defer i.detailsMutex.Unlock()
	return i.full, i.full != nil
}

// DetailsLoaded tells us whether we've yet fetched the full details for this
// instance.
func (i *Instance) DetailsLoaded() bool {
	_, ok := i.Full()
	return ok
}

// IsVM returns true if the instance is a virtual machine rather than a container.
func (i *Instance) IsVM() bool {
	return i.Instance.Type == "virtual-machine"
}

func (i *Instance) updateState(action string, timeout int, force bool) error {
	i.Log.Warn(fmt.Sprintf("%s instance %s", action, i.Name))
	op, err := i.Client.UpdateInstanceState(i.Name, api.InstanceStatePut{
		Action:  action,
		Timeout: timeout,
		Force:   force,
	}, "")
	if err != nil {
		return err
	}
	return op.Wait()
}

// Start starts the instance.
func (i *Instance) Start() error {
	return i.updateState("start", -1, false)
}

// Stop stops the instance.
func (i *Instance) Stop() error {
	return i.updateState("stop", 30, false)
}

// Restart restarts the instance.
func (i *Instance) Restart() error {
	return i.updateState("restart", 30, false)
}

// Freeze pauses (freezes) the instance. Only supported for containers.
func (i *Instance) Freeze() error {
	return i.updateState("freeze", -1, false)
}

// Unfreeze resumes a frozen instance.
func (i *Instance) Unfreeze() error {
	return i.updateState("unfreeze", -1, false)
}

// ErrInstanceRunning is what Delete returns when Incus refused to delete the
// instance because it was still running. Callers can catch this to offer a
// force-stop-then-delete flow (see ForceDelete).
var ErrInstanceRunning = errors.New("instance is running")

// Delete deletes the instance. Incus refuses to delete an instance that isn't
// stopped, in which case this returns ErrInstanceRunning; use ForceDelete to
// stop it first.
func (i *Instance) Delete() error {
	i.Log.Warn(fmt.Sprintf("deleting instance %s", i.Name))
	op, err := i.Client.DeleteInstance(i.Name)
	if err != nil {
		return asDeleteError(err)
	}

	return asDeleteError(op.Wait())
}

// ForceDelete stops the instance without waiting for a clean shutdown and then
// deletes it, mirroring `incus delete --force` (see deleteOne in
// cmd/incus/delete.go).
func (i *Instance) ForceDelete() error {
	i.Log.Warn(fmt.Sprintf("force stopping instance %s before deleting it", i.Name))
	if err := i.updateState("stop", -1, true); err != nil {
		return fmt.Errorf("stopping the instance failed: %w", err)
	}

	// Incus discards ephemeral instances the moment they stop, so there's
	// nothing left to delete and asking would just 404. `incus delete --force`
	// returns early here too.
	if i.Instance.Ephemeral {
		return nil
	}

	return i.Delete()
}

// asDeleteError translates the daemon's refusal to delete a running instance
// into ErrInstanceRunning. incusd rejects it with a plain 400 whose body is
// "Instance is running" (instanceDelete in cmd/incusd/instance_delete.go) and
// no dedicated error code, so the message is all there is to match on. An
// IsRunning() pre-check wouldn't do: the daemon counts frozen as running.
func asDeleteError(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "instance is running") {
		return ErrInstanceRunning
	}

	return err
}

// IsRunning tells us whether Incus considers this instance running.
func (i *Instance) IsRunning() bool {
	return strings.EqualFold(i.Instance.Status, "Running")
}

// Addresses returns the instance's global-scope IP addresses for an
// api.InstanceStateNetworkAddress.Family ("inet"/"inet6"), sorted and
// excluding loopback. Empty until RefreshInstanceDetails has run.
func (i *Instance) Addresses(family string) []string {
	full, ok := i.Full()
	if !ok || full.State == nil {
		return nil
	}

	addresses := []string{}
	for name, network := range full.State.Network {
		if name == "lo" {
			continue
		}
		for _, addr := range network.Addresses {
			if addr.Scope != "global" || addr.Family != family {
				continue
			}
			addresses = append(addresses, addr.Address)
		}
	}

	sort.Strings(addresses)

	return addresses
}

// ConsoleLog returns one raw fetch of the console log. The endpoint drains
// on read, so each call returns only what buffered since the last one -
// anything polling repeatedly wants TailConsoleLog instead.
func (i *Instance) ConsoleLog() (string, error) {
	reader, err := i.Client.GetInstanceConsoleLog(i.Name, &incus.InstanceConsoleLogArgs{})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// TailConsoleLog accumulates successive ConsoleLog fetches into a capped
// per-instance buffer, giving pollers a stable growing view.
//
// Drain-on-read only holds while the instance runs; once stopped, incusd
// serves the whole persisted log file on every request, so we fetch once
// after a stop and then leave the buffer alone - otherwise every tick
// re-appends the entire log. A fetch error returns the last-known-good
// buffer rather than clearing it, so a transient failure doesn't blank out
// logs already on screen.
func (i *Instance) TailConsoleLog() (string, error) {
	if !i.IsRunning() {
		i.logMutex.Lock()
		alreadyFetched := i.stoppedLogFetched
		buffered := i.logBuffer.String()
		i.logMutex.Unlock()
		if alreadyFetched {
			return buffered, nil
		}
	}

	reader, err := i.Client.GetInstanceConsoleLog(i.Name, &incus.InstanceConsoleLogArgs{})
	if err == nil {
		var data []byte
		data, err = io.ReadAll(reader)
		reader.Close()
		if err == nil && len(data) > 0 {
			i.appendToLogBuffer(data)
		}
	}

	i.logMutex.Lock()
	i.stoppedLogFetched = !i.IsRunning()
	buffered := i.logBuffer.String()
	i.logMutex.Unlock()

	return buffered, err
}

func (i *Instance) appendToLogBuffer(data []byte) {
	i.logMutex.Lock()
	defer i.logMutex.Unlock()

	i.logBuffer.Write(data)

	if i.logBuffer.Len() > maxConsoleLogBufferBytes {
		trimmed := i.logBuffer.String()
		trimmed = trimmed[len(trimmed)-maxConsoleLogBufferBytes:]
		i.logBuffer.Reset()
		i.logBuffer.WriteString(trimmed)
	}
}
