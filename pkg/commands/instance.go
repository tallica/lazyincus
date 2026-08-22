package commands

import (
	"fmt"
	"io"
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

// Delete deletes the instance. Incus refuses to delete a running instance, so
// callers should stop it first (or catch the error and offer to force-stop).
func (i *Instance) Delete() error {
	i.Log.Warn(fmt.Sprintf("deleting instance %s", i.Name))
	op, err := i.Client.DeleteInstance(i.Name)
	if err != nil {
		return err
	}
	return op.Wait()
}

// IsRunning tells us whether Incus considers this instance running.
func (i *Instance) IsRunning() bool {
	return strings.EqualFold(i.Instance.Status, "Running")
}

// ConsoleLog returns the current contents of the instance's console log ring
// buffer as returned by a single fetch. This is a raw snapshot: despite the
// daemon requesting ClearLog: false, each HTTP read of Incus's console log
// endpoint appears to drain newly-buffered bytes (much like reading a FIFO)
// rather than peeking at accumulated content - confirmed directly against
// the `incus` CLI, not just this client: `incus console <name> --show-log`
// run twice in a row shows real output on the first call and nothing on the
// second, even though the instance kept running and (for a busy service
// like nginx) kept producing output. Prefer TailConsoleLog for anything
// that polls repeatedly, since a naive "replace displayed content with the
// latest snapshot" loop built on this method will flicker to empty on every
// tick where nothing new happened to be buffered.
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

// TailConsoleLog fetches the latest console log chunk and appends it to a
// per-instance in-memory buffer, returning the accumulated content so far.
// This gives repeated callers (the Logs tab's polling loop) a stable,
// growing view despite the underlying endpoint's drain-on-read behavior -
// see ConsoleLog's doc comment for why a plain repeated ConsoleLog call
// doesn't work for that.
//
// That drain-on-read behavior only holds while the instance is actually
// running: incusd reads the live console ring buffer in that case, but
// once an instance is stopped it instead serves the persisted log file
// as-is on every request - the same content back every time, not fresh
// bytes. Naively re-fetching and re-appending every poll tick would flood
// the buffer with duplicate messages once stopped, so once IsRunning() is
// false we only fetch once (to pick up any final output) and then leave
// the buffer alone until the instance starts running again -
// GetInstanceConsoleLog doesn't expose response headers (e.g.
// Last-Modified) through this client library to check staleness any
// other way.
//
// A fetch error doesn't clear or replace the buffer - it's returned
// alongside the last-known-good accumulated content, so a transient poll
// failure doesn't blank out logs that were already visible.
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
