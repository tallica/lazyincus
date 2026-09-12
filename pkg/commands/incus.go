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
	ErrorChan     chan error
	InstanceMutex deadlock.Mutex

	// RemoteName is the Incus remote we connected to (e.g. "local", or a
	// colima/lima remote name), taken from the CLI config's default-remote.
	RemoteName string
	// ServerVersion and ServerName are fetched once at connect time via
	// GetServer(); empty if that call failed.
	ServerVersion string
	ServerName    string

	// Guarded by clientMutex: both change when the user switches project.
	clientMutex deadlock.Mutex
	client      incus.InstanceServer
	projectName string

	connMutex deadlock.Mutex
	connected bool

	Closers []io.Closer
}

var _ io.Closer = &IncusCommand{}

// LimitedIncusCommand is a stripped-down IncusCommand with just the methods that
// an Instance might need. Kept as an interface (mirroring lazydocker's design)
// so that Instance doesn't need to import the whole command package.
type LimitedIncusCommand interface{}

// NewIncusCommand connects to the CLI's configured default remote via
// Incus's own cliconfig. Resolving the socket path ourselves would miss
// setups where the daemon runs in a VM (colima on macOS), whose socket
// lives at a path recorded in the remote's config.
func NewIncusCommand(log *logrus.Entry, osCommand *OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig, errorChan chan error) (*IncusCommand, error) {
	cliCfg, err := cliconfig.LoadConfig("")
	if err != nil {
		return nil, err
	}

	client, err := cliCfg.GetInstanceServer(cliCfg.DefaultRemote)
	if err != nil {
		return nil, err
	}

	command := &IncusCommand{
		Log:         log,
		OSCommand:   osCommand,
		Tr:          tr,
		Config:      cfg,
		client:      client,
		ErrorChan:   errorChan,
		RemoteName:  cliCfg.DefaultRemote,
		projectName: clientProjectName(client),
		connected:   true,
	}

	// Best-effort: a failed GetServer() shouldn't prevent startup, since
	// the instance list is what actually matters. The footer just shows
	// no version if this fails.
	if server, _, err := client.GetServer(); err == nil {
		command.ServerVersion = server.Environment.ServerVersion
		command.ServerName = server.Environment.ServerName
	} else {
		log.Warn(err)
	}

	return command, nil
}

// clientProjectName reports the project a client is scoped to. An empty
// project - a client that never had UseProject called on it - means default.
func clientProjectName(client incus.InstanceServer) string {
	info, err := client.GetConnectionInfo()
	if err != nil || info.Project == "" {
		return api.ProjectDefaultName
	}

	return info.Project
}

// Client returns the current instance server. A method rather than a field
// because switching project swaps it out from under whoever holds it.
func (c *IncusCommand) Client() incus.InstanceServer {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	return c.client
}

// ProjectName is the Incus project the instance list is currently scoped to.
func (c *IncusCommand) ProjectName() string {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	return c.projectName
}

// GetProjectNames lists the projects on the server.
func (c *IncusCommand) GetProjectNames() ([]string, error) {
	return c.Client().GetProjectNames()
}

// UseProject re-scopes the client to the given project. Purely local: the
// client just carries a different project in its requests, so nothing fails.
func (c *IncusCommand) UseProject(name string) {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	c.client = c.client.UseProject(name)
	c.projectName = name
}

func (c *IncusCommand) Close() error {
	return utils.CloseMany(c.Closers)
}

// IsConnected reports whether the most recent request to the daemon
// succeeded. Updated by GetInstances, which runs on a background poll, so
// this reflects connection health without a dedicated heartbeat.
func (c *IncusCommand) IsConnected() bool {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	return c.connected
}

func (c *IncusCommand) setConnected(connected bool) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	c.connected = connected
}

// GetInstances lists all instances (containers and VMs). Existing Instance
// objects are reused (by name) so that any cached details survive a refresh.
func (c *IncusCommand) GetInstances(existingInstances []*Instance) ([]*Instance, error) {
	c.InstanceMutex.Lock()
	defer c.InstanceMutex.Unlock()

	client := c.Client()

	apiInstances, err := client.GetInstances(api.InstanceTypeAny)
	if err != nil {
		c.setConnected(false)
		return nil, err
	}
	c.setConnected(true)

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
				OSCommand:    c.OSCommand,
				Log:          c.Log,
				IncusCommand: c,
				Tr:           c.Tr,
			}
		}

		// Reassigned every refresh, not just at construction: an instance
		// reused by name across a project switch would otherwise keep a
		// client pointed at the project it came from.
		inst.Client = client
		inst.Instance = apiInstance
		ownInstances[i] = inst
	}

	return ownInstances, nil
}

// GetImages lists the images stored on the server, reusing existing Image
// objects by fingerprint the way GetInstances does.
func (c *IncusCommand) GetImages(existingImages []*Image) ([]*Image, error) {
	client := c.Client()

	apiImages, err := client.GetImages()
	if err != nil {
		c.setConnected(false)
		return nil, err
	}
	c.setConnected(true)

	ownImages := make([]*Image, len(apiImages))

	for i := range apiImages {
		apiImage := apiImages[i]

		var image *Image
		for _, existing := range existingImages {
			if existing.Fingerprint == apiImage.Fingerprint {
				image = existing
				break
			}
		}

		if image == nil {
			image = &Image{
				Fingerprint: apiImage.Fingerprint,
				OSCommand:   c.OSCommand,
				Log:         c.Log,
				Tr:          c.Tr,
			}
		}

		image.Client = client
		image.Image = apiImage
		ownImages[i] = image
	}

	return ownImages, nil
}

// RefreshInstanceDetails fetches the full details (including state) for each
// instance in the background.
func (c *IncusCommand) RefreshInstanceDetails(instances []*Instance) {
	client := c.Client()

	for _, inst := range instances {
		inst := inst
		go func() {
			full, _, err := client.GetInstanceFull(inst.Name)
			if err != nil {
				c.Log.Warn(err)
				return
			}
			inst.setFull(full)
		}()
	}
}
