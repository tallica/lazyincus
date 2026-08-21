package commands

import (
	"io"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/cliconfig"
	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// IncusCommand is our main interface into the Incus API
type IncusCommand struct {
	Log           *logrus.Entry
	OSCommand     *OSCommand
	Tr            *i18n.TranslationSet
	Config        *config.AppConfig
	Client        incus.InstanceServer
	ErrorChan     chan error
	InstanceMutex deadlock.Mutex

	Closers []io.Closer
}

var _ io.Closer = &IncusCommand{}

// LimitedIncusCommand is a stripped-down IncusCommand with just the methods that
// an Instance might need. Kept as an interface (mirroring lazydocker's design)
// so that Instance doesn't need to import the whole command package.
type LimitedIncusCommand interface{}

// NewIncusCommand connects to Incus using the same remote-resolution logic
// as the `incus` CLI itself: it loads ~/.config/incus/config.yml (or the
// platform equivalent, or $INCUS_CONF) and connects to the configured
// default remote.
//
// This matters beyond Linux hosts running incusd directly: on macOS/Windows
// setups (e.g. `colima start --runtime incus`), the daemon runs inside a VM
// and the "local" unix socket lives at a path recorded in that remote's
// config (e.g. unix:///Users/you/.colima/default/incus.sock), not at any of
// the standard Linux socket locations. Reimplementing just the bare
// $INCUS_SOCKET/$INCUS_DIR/default-path resolution (as an earlier version of
// this function did) misses that case entirely, so we defer to Incus's own
// shared/cliconfig package, which already knows how to resolve remotes,
// unix-socket paths, and TLS-authenticated connections consistently with
// the CLI.
func NewIncusCommand(log *logrus.Entry, osCommand *OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig, errorChan chan error) (*IncusCommand, error) {
	cliCfg, err := cliconfig.LoadConfig("")
	if err != nil {
		return nil, err
	}

	client, err := cliCfg.GetInstanceServer(cliCfg.DefaultRemote)
	if err != nil {
		return nil, err
	}

	return &IncusCommand{
		Log:       log,
		OSCommand: osCommand,
		Tr:        tr,
		Config:    cfg,
		Client:    client,
		ErrorChan: errorChan,
	}, nil
}

func (c *IncusCommand) Close() error {
	return utils.CloseMany(c.Closers)
}

// GetInstances lists all instances (containers and VMs). Existing Instance
// objects are reused (by name) so that any cached details survive a refresh.
func (c *IncusCommand) GetInstances(existingInstances []*Instance) ([]*Instance, error) {
	c.InstanceMutex.Lock()
	defer c.InstanceMutex.Unlock()

	apiInstances, err := c.Client.GetInstances(api.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	ownInstances := make([]*Instance, len(apiInstances))

	for i := range apiInstances {
		apiInstance := apiInstances[i]

		var inst *Instance
		for _, existing := range existingInstances {
			if existing.Name == apiInstance.Name {
				inst = existing
				break
			}
		}

		if inst == nil {
			inst = &Instance{
				Name:         apiInstance.Name,
				Client:       c.Client,
				OSCommand:    c.OSCommand,
				Log:          c.Log,
				IncusCommand: c,
				Tr:           c.Tr,
			}
		}

		inst.Instance = apiInstance
		ownInstances[i] = inst
	}

	return ownInstances, nil
}

// RefreshInstanceDetails fetches the full details (including state) for each
// instance in the background.
func (c *IncusCommand) RefreshInstanceDetails(instances []*Instance) {
	for _, inst := range instances {
		inst := inst
		go func() {
			full, _, err := c.Client.GetInstanceFull(inst.Name)
			if err != nil {
				c.Log.Warn(err)
				return
			}
			inst.setFull(full)
		}()
	}
}
