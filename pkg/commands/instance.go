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
}

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
// buffer. This is the closest Incus analog to `docker logs`: Incus doesn't
// capture arbitrary stdout/stderr the way the Docker daemon does, but it does
// keep a ring buffer of console output for both containers and VMs.
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
