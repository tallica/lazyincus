package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/util"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Instance represents an Incus instance (either a container or a VM).
type Instance struct {
	Name string

	// Project the instance lives in. Only interesting in the all-projects
	// view, where the list mixes projects; the CLI shell-outs need it to
	// address the right instance.
	Project string

	// Instance is the daemon's whole view of the instance as of the refresh
	// that built this value: config, state and snapshots. Never updated in
	// place - the next refresh builds a new Instance; see Latest.
	Instance api.InstanceFull

	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet

	runtime *instanceRuntime
}

// maxConsoleLogBufferBytes caps how much accumulated console output
// TailConsoleLog keeps per instance, to bound memory for long-running
// sessions.
const maxConsoleLogBufferBytes = 256 * 1024

// Key is the instance's identity: names repeat across projects.
func (i *Instance) Key() string {
	return i.Project + "/" + i.Name
}

// Latest is the newest refresh's view of this same instance, for whatever
// outlives the refresh that built this one - a main-panel tab that ticks.
// The instance itself once a later refresh no longer lists it.
func (i *Instance) Latest() *Instance {
	if i.runtime == nil {
		return i
	}

	i.runtime.mutex.Lock()
	defer i.runtime.mutex.Unlock()

	return i.runtime.latest
}

// IsVM returns true if the instance is a virtual machine rather than a container.
func (i *Instance) IsVM() bool {
	return i.Instance.Type == "virtual-machine"
}

// ociContainerKey is what the `incus` CLI reads to mark a container as an
// OCI application container (typeColumnData in cmd/incus/list.go).
const ociContainerKey = "volatile.container.oci"

// IsOCI reports whether this is an OCI application container - one running
// an image's entrypoint rather than an init system.
func (i *Instance) IsOCI() bool {
	return util.IsTrue(i.config(ociContainerKey))
}

func (i *Instance) updateState(action string, timeout int, force bool) error {
	i.Log.Warn(fmt.Sprintf("%s instance %s", action, i.Name))

	return i.retryWhileBusy(func() error {
		op, err := i.Client.UpdateInstanceState(i.Name, api.InstanceStatePut{
			Action:  action,
			Timeout: timeout,
			Force:   force,
		}, "")
		if err != nil {
			return err
		}

		return op.Wait()
	})
}

// busyMessage is how incusd refuses a request while another operation holds
// the instance: `Instance is busy running a "update" operation`
// (instanceOperationLock in internal/server/instance/operationlock). There's
// no error code for it, so the message is the test, the way asDeleteError
// matches the running-instance refusal.
const busyMessage = "is busy running"

// retryWhileBusy rides out another writer holding the instance: a compose
// stack with healthchecks has ic-healthd stamping a verdict into every
// instance's config on a timer, and what's in the way is milliseconds long.
func (i *Instance) retryWhileBusy(request func() error) error {
	const (
		window = 3 * time.Second
		pause  = 150 * time.Millisecond
	)

	deadline := time.Now().Add(window)

	for {
		err := request()
		if err == nil || !strings.Contains(err.Error(), busyMessage) || time.Now().After(deadline) {
			return err
		}

		i.Log.Warn(fmt.Sprintf("instance %s is busy, retrying: %v", i.Name, err))
		time.Sleep(pause)
	}
}

// Start starts the instance.
func (i *Instance) Start() error {
	if i.isCompose() {
		return i.composeStart()
	}

	return i.updateState("start", -1, false)
}

// Stop stops the instance.
func (i *Instance) Stop() error {
	if i.isCompose() {
		return i.composeStop(composeStopTimeout)
	}

	return i.updateState("stop", 30, false)
}

// Restart restarts the instance.
func (i *Instance) Restart() error {
	if i.isCompose() {
		return i.composeRestart()
	}

	return i.updateState("restart", 30, false)
}

// Freeze pauses (freezes) the instance. Only supported for containers.
func (i *Instance) Freeze() error {
	freeze := func() error { return i.updateState("freeze", -1, false) }
	if i.isCompose() {
		return i.whileMarkedStopped(api.Frozen, freeze)
	}

	return freeze()
}

// Unfreeze resumes a frozen instance.
func (i *Instance) Unfreeze() error {
	if i.isCompose() {
		return i.composeUnfreeze()
	}

	return i.updateState("unfreeze", -1, false)
}

// ForceStop stops the instance without waiting for a clean shutdown,
// mirroring `incus stop --force`.
func (i *Instance) ForceStop() error {
	kill := func() error { return i.updateState("stop", -1, true) }
	if i.isCompose() {
		return i.whileMarkedStopped(api.Stopped, kill)
	}

	return kill()
}

// ErrInstanceRunning is what Delete returns when Incus refused to delete the
// instance because it was still running. Callers can catch this to offer a
// force-stop-then-delete flow (see ForceDelete).
var ErrInstanceRunning = errors.New("instance is running")

// ErrInstanceNotRunning is returned by operations that need a running
// instance, such as listing its processes.
var ErrInstanceNotRunning = errors.New("instance is not running")

// Delete deletes the instance. Incus refuses to delete an instance that isn't
// stopped, in which case this returns ErrInstanceRunning; use ForceDelete to
// stop it first.
func (i *Instance) Delete() error {
	i.Log.Warn(fmt.Sprintf("deleting instance %s", i.Name))

	return i.retryWhileBusy(func() error {
		op, err := i.Client.DeleteInstance(i.Name)
		if err != nil {
			return asDeleteError(err)
		}

		return asDeleteError(op.Wait())
	})
}

// ForceDelete stops the instance without waiting for a clean shutdown and then
// deletes it, mirroring `incus delete --force` (see deleteOne in
// cmd/incus/delete.go).
func (i *Instance) ForceDelete() error {
	i.Log.Warn(fmt.Sprintf("force stopping instance %s before deleting it", i.Name))
	if err := i.ForceStop(); err != nil {
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

// composeServiceKey is the label incus-compose stamps on every instance it
// creates, naming the compose service the instance came from (see
// project/instance.go in lxc/incus-compose). It's a plain user config key
// with no compatibility promise, so nothing here depends on it being set.
const composeServiceKey = "user.label.incus-compose.service"

// ComposeService is the incus-compose service this instance belongs to, or
// an empty string for an instance nobody created through compose.
func (i *Instance) ComposeService() string {
	return i.config(composeServiceKey)
}

// healthStatusKey is where ic-healthd writes its verdict; the opt-in key can
// sit on the project, which ExpandedConfig doesn't expand.
const healthStatusKey = "user.healthcheck.status"

// The values ic-healthd writes to healthStatusKey (shared/health.go).
const (
	HealthUnknown   = "unknown"
	HealthStarting  = "starting"
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthStopped   = "stopped"
)

// HealthStatus is the instance's ic-healthd health, empty when unchecked.
func (i *Instance) HealthStatus() string {
	return i.config(healthStatusKey)
}

const (
	composeImageKey     = "user.image_alias"
	imageDescriptionKey = "image.description"
	baseImageKey        = "volatile.base_image"
)

// ComposeImage is the image reference the compose file named.
func (i *Instance) ComposeImage() string {
	return i.config(composeImageKey)
}

// Image is what the instance was created from, the way the Images panel
// labels the same thing: the compose file's reference when there is one,
// else the description the image left in the instance's config ("Alpine
// 3.21 arm64 (20260825_13:00)"), else the short fingerprint of the image
// itself, for one that carried no description.
func (i *Instance) Image() string {
	if image := i.ComposeImage(); image != "" {
		return image
	}

	if description := i.config(imageDescriptionKey); description != "" {
		return description
	}

	fingerprint := i.config(baseImageKey)
	if len(fingerprint) < shortFingerprintLength {
		return fingerprint
	}

	return fingerprint[:shortFingerprintLength]
}

func (i *Instance) config(key string) string {
	return i.Instance.ExpandedConfig[key]
}

// Addresses returns the instance's global-scope IP addresses for an
// api.InstanceStateNetworkAddress.Family ("inet"/"inet6"), sorted and
// excluding loopback.
func (i *Instance) Addresses(family string) []string {
	if i.Instance.State == nil {
		return nil
	}

	addresses := []string{}
	for name, network := range i.Instance.State.Network {
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

// TailConsoleLog accumulates successive console-log fetches into a capped
// per-instance buffer, giving pollers a stable growing view. It reads the
// newest refresh's status, a tab that tails the log outliving the refresh
// that opened it.
//
// Drain-on-read only holds while the instance runs; once stopped, incusd
// serves the whole persisted log file on every request, so we fetch once
// after a stop and then leave the buffer alone - otherwise every tick
// re-appends the entire log. A fetch error returns the last-known-good
// buffer rather than clearing it, so a transient failure doesn't blank out
// logs already on screen.
func (i *Instance) TailConsoleLog() (string, error) {
	latest := i.Latest()
	runtime := latest.runtimeOrOwn()

	running := latest.IsRunning()

	runtime.mutex.Lock()
	alreadyFetched := runtime.stoppedLogFetched
	buffered := runtime.logBuffer.String()
	runtime.mutex.Unlock()

	if !running && alreadyFetched {
		return buffered, nil
	}

	reader, err := latest.Client.GetInstanceConsoleLog(latest.Name, &incus.InstanceConsoleLogArgs{})
	if err == nil {
		var data []byte
		data, err = io.ReadAll(reader)
		reader.Close()
		if err == nil && len(data) > 0 {
			runtime.appendToLog(data)
		}
	}

	runtime.mutex.Lock()
	runtime.stoppedLogFetched = !running
	buffered = runtime.logBuffer.String()
	runtime.mutex.Unlock()

	return buffered, err
}

// runtimeOrOwn is the instance's runtime, or one of its own for an instance
// no listing attached - a test's.
func (i *Instance) runtimeOrOwn() *instanceRuntime {
	if i.runtime == nil {
		i.runtime = &instanceRuntime{latest: i}
	}

	return i.runtime
}

func (r *instanceRuntime) appendToLog(data []byte) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.logBuffer.Write(data)

	if r.logBuffer.Len() > maxConsoleLogBufferBytes {
		trimmed := r.logBuffer.String()
		trimmed = trimmed[len(trimmed)-maxConsoleLogBufferBytes:]
		r.logBuffer.Reset()
		r.logBuffer.WriteString(trimmed)
	}
}

// topCommands are tried in order until one exits cleanly with output. Full
// `ps` flags aren't portable: busybox's ps (Alpine, most OCI images) ignores
// BSD-style options and prints its own fixed columns, while util-linux's
// needs them to show anything beyond the current terminal's processes.
var topCommands = [][]string{
	{"ps", "-eo", "pid,user,pcpu,pmem,args"},
	{"ps", "aux"},
	{"ps"},
}

// Top lists the processes running inside the instance. Incus's API only
// reports a process count, so this execs `ps` in the instance itself -
// which needs the guest agent on a VM, and a `ps` binary in the image.
func (i *Instance) Top() (string, error) {
	latest := i.Latest()
	if !latest.IsRunning() {
		return "", ErrInstanceNotRunning
	}

	return latest.top()
}

func (i *Instance) top() (string, error) {
	var lastErr error

	for _, command := range i.candidateTopCommands() {
		output, err := i.exec(command)
		if err != nil {
			lastErr = err
			continue
		}

		if strings.TrimSpace(output) != "" {
			i.setTopCommand(command)
			return output, nil
		}
	}

	i.setTopCommand(nil)

	if lastErr != nil {
		return "", lastErr
	}

	return "", nil
}

func (i *Instance) candidateTopCommands() [][]string {
	runtime := i.runtimeOrOwn()

	runtime.mutex.Lock()
	cached := runtime.topCommand
	runtime.mutex.Unlock()

	if cached == nil {
		return topCommands
	}

	return append([][]string{cached}, topCommands...)
}

func (i *Instance) setTopCommand(command []string) {
	runtime := i.runtimeOrOwn()

	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	runtime.topCommand = command
}

// exec runs a command in the instance and returns its stdout, using the
// client library's websocket exec rather than the `incus` CLI: this runs on
// a poll, so spawning a process per tick would be wasteful, and nothing here
// needs a terminal attached.
func (i *Instance) exec(command []string) (string, error) {
	var stdout, stderr bytes.Buffer

	dataDone := make(chan bool)

	op, err := i.Client.ExecInstance(i.Name, api.InstanceExecPost{
		Command:   command,
		WaitForWS: true,
	}, &incus.InstanceExecArgs{
		Stdout:   &stdout,
		Stderr:   &stderr,
		DataDone: dataDone,
	})
	if err != nil {
		return "", err
	}

	if err := op.Wait(); err != nil {
		return "", err
	}

	// The websockets carrying stdout/stderr outlive the operation itself, so
	// the buffers aren't complete until this closes.
	<-dataDone

	if code, ok := op.Get().Metadata["return"].(float64); ok && code != 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("%s exited with status %d", command[0], int(code))
		}

		return "", errors.New(message)
	}

	return stdout.String(), nil
}
