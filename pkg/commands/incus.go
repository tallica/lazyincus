package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/cliconfig"
	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// IncusCommand is the app's way into one Incus remote: the session's, or
// another a stack is on.
type IncusCommand struct {
	Log       *logrus.Entry
	OSCommand *OSCommand
	Tr        *i18n.TranslationSet
	Config    *config.AppConfig

	// Guarded by clientMutex: they change when the user switches project,
	// and all of them when the user switches remote.
	clientMutex deadlock.Mutex
	client      incus.InstanceServer
	projectName string
	allProjects bool
	// remoteName is the CLI remote client is connected to, serverVersion
	// what its GetServer() said at connect time, empty if that failed.
	remoteName    string
	serverVersion string

	connMutex deadlock.Mutex
	connected bool

	runtimes instanceRuntimes

	// cliCfg is the CLI config the client came from, which connects to the
	// other remotes a stack can be pinned to. Nil for incustest's.
	cliCfg *cliconfig.Config
}

// dialTimeout bounds the connection attempt, which the client otherwise
// leaves to the OS - a minute or more on a remote that has gone away.
const dialTimeout = 5 * time.Second

// connectTimeout is the same bound for connecting at all - at startup, on
// `R`, or to a stack's remote. cliconfig calls
// GetServer() before handing back a client, so that first request is made
// on a transport we don't have yet, and the call is the only thing left to
// bound.
const connectTimeout = 10 * time.Second

// NewIncusCommand connects to the CLI's configured default remote via
// Incus's own cliconfig. Resolving the socket path ourselves would miss
// every remote that doesn't keep it where we'd look - a daemon in a VM
// records its own path in the remote's config.
func NewIncusCommand(log *logrus.Entry, osCommand *OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig) (*IncusCommand, error) {
	cliCfg, err := cliconfig.LoadConfig("")
	if err != nil {
		return nil, &ConnectError{Err: err}
	}

	return connectCommand(log, osCommand, tr, cfg, cliCfg, cliCfg.DefaultRemote)
}

// ConnectRemote is a command like c on another of the CLI's remotes.
func (c *IncusCommand) ConnectRemote(name string) (*IncusCommand, error) {
	if c.cliCfg == nil {
		return nil, &ConnectError{Remote: name, Err: errors.New("no CLI config")}
	}

	return connectCommand(c.Log, c.OSCommand, c.Tr, c.Config, c.cliCfg, name)
}

// IsInstanceRemote is whether name is one of the CLI's remotes that has
// instances: `images:` and OCI registries are remotes too.
func (c *IncusCommand) IsInstanceRemote(name string) bool {
	if c.cliCfg == nil {
		return false
	}

	remote, ok := c.cliCfg.Remotes[name]

	return ok && !remote.Public && remote.Protocol == "incus"
}

// InstanceRemoteNames is every remote IsInstanceRemote takes, in name
// order, as `incus remote list` shows them: `local` too where cliconfig
// refuses it, off Linux, which switching to says.
func (c *IncusCommand) InstanceRemoteNames() []string {
	if c.cliCfg == nil {
		return nil
	}

	names := []string{}

	for name := range c.cliCfg.Remotes {
		if c.IsInstanceRemote(name) {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names
}

func connectCommand(log *logrus.Entry, osCommand *OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig, cliCfg *cliconfig.Config, name string) (*IncusCommand, error) {
	client, err := connectRemote(cliCfg, name)
	if err != nil {
		return nil, &ConnectError{Remote: name, Err: err}
	}

	capDialTimeout(client)

	command := NewIncusCommandWithClient(log, osCommand, tr, cfg, client, name)
	command.cliCfg = cliCfg

	return command, nil
}

// NewIncusCommandWithClient is an IncusCommand around a client already
// connected - to a remote, or to incustest's stand-in.
func NewIncusCommandWithClient(log *logrus.Entry, osCommand *OSCommand, tr *i18n.TranslationSet, cfg *config.AppConfig, client incus.InstanceServer, remote string) *IncusCommand {
	// Best-effort: the footer just shows no version without it.
	version := ""
	if server, _, err := client.GetServer(); err == nil {
		version = server.Environment.ServerVersion
	} else if log != nil {
		log.Warn(err)
	}

	return &IncusCommand{
		Log:           log,
		OSCommand:     osCommand,
		Tr:            tr,
		Config:        cfg,
		client:        client,
		remoteName:    remote,
		serverVersion: version,
		projectName:   clientProjectName(client),
		// Every project by default: a server with one project looks the same
		// either way, and on a server with several, scoping to whichever one
		// the user's remote happens to point at hides the rest with no hint
		// that they're there.
		allProjects: true,
		connected:   true,
	}
}

func clientURL(client incus.InstanceServer) string {
	info, err := client.GetConnectionInfo()
	if err != nil {
		return ""
	}

	return info.URL
}

func publishHost(remoteURL string) string {
	remote, err := url.Parse(remoteURL)
	if err != nil || remote.Scheme == "unix" {
		return ""
	}

	host := remote.Hostname()
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return ""
	}

	return host
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

// connectRemote is GetInstanceServer under connectTimeout. The goroutine
// outlives a timeout, left to finish or fail on its own.
func connectRemote(cliCfg *cliconfig.Config, name string) (incus.InstanceServer, error) {
	type result struct {
		client incus.InstanceServer
		err    error
	}

	done := make(chan result, 1)

	go func() {
		client, err := cliCfg.GetInstanceServer(name)
		done <- result{client: client, err: err}
	}()

	select {
	case res := <-done:
		return res.client, res.err
	case <-time.After(connectTimeout):
		return nil, fmt.Errorf("no answer within %s", connectTimeout)
	}
}

// capDialTimeout bounds the dial alone, not the request: a console log or
// an image transfer takes as long as it takes once the daemon is answering.
func capDialTimeout(client incus.InstanceServer) {
	httpClient, err := client.GetHTTPClient()
	if err != nil {
		return
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		return
	}

	capTransportDial(transport)
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// capTransportDial covers both dialers: the client sets DialContext for a
// unix socket and DialTLSContext for a TLS remote, never both.
func capTransportDial(transport *http.Transport) {
	transport.DialContext = withDialTimeout(transport.DialContext)
	transport.DialTLSContext = withDialTimeout(transport.DialTLSContext)
}

func withDialTimeout(dial dialFunc) dialFunc {
	if dial == nil {
		return nil
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()

		return dial(ctx, network, addr)
	}
}

// RemoteName is the CLI remote the command is connected to.
func (c *IncusCommand) RemoteName() string {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	return c.remoteName
}

// ServerVersion is the daemon's version, empty when it didn't say.
func (c *IncusCommand) ServerVersion() string {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	return c.serverVersion
}

// PublishHost is where a port the daemon's host publishes is reached from
// here - the remote's own host - or empty when its URL doesn't say: a unix
// socket, or loopback, which is a tunnel to the API alone.
func (c *IncusCommand) PublishHost() string {
	return publishHost(clientURL(c.Client()))
}

// UseRemote moves c onto other's connection, listing every project there:
// the new server's projects have nothing to do with the old one's. c keeps
// its identity, so whoever holds it follows.
func (c *IncusCommand) UseRemote(other *IncusCommand) {
	client, project, _ := other.scope()
	remote, version := other.RemoteName(), other.ServerVersion()

	c.clientMutex.Lock()
	c.client, c.projectName, c.allProjects = client, project, true
	c.remoteName, c.serverVersion = remote, version
	c.clientMutex.Unlock()

	c.setConnected(true)
	// An instance on the new remote can share a name with one on the old.
	c.runtimes.reset()
}

// Client returns the current instance server. A method rather than a field
// because switching project swaps it out from under whoever holds it.
func (c *IncusCommand) Client() incus.InstanceServer {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	return c.client
}

// ProjectName is the Incus project the panels are currently scoped to, or
// an empty string when they're showing every project.
func (c *IncusCommand) ProjectName() string {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	if c.allProjects {
		return ""
	}

	return c.projectName
}

// IsAllProjects reports whether the panels are listing every project rather
// than one. Actions still run against the project each item came from - see
// clientFor.
func (c *IncusCommand) IsAllProjects() bool {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	return c.allProjects
}

// UseAllProjects lists every project at once. The client itself stays
// scoped to whatever project it had: the all-projects endpoints ignore that
// scope, and per-item actions need a client scoped to the item's own
// project anyway.
func (c *IncusCommand) UseAllProjects() {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	c.allProjects = true
}

// clientFor returns a client scoped to the given project, so an action on an
// item from an all-projects listing goes to the project that item lives in
// rather than whichever one the client happens to be scoped to.
func (c *IncusCommand) clientFor(project string) incus.InstanceServer {
	client := c.Client()

	if project == "" || !c.IsAllProjects() {
		return client
	}

	return client.UseProject(project)
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
	c.allProjects = false
}

// IsConnected reports whether the daemon is reachable, as of the most
// recent request. Updated by the list calls, which run on a background
// poll, so this reflects connection health without a dedicated heartbeat.
func (c *IncusCommand) IsConnected() bool {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	return c.connected
}

// NoteError records what an error says about the connection. Anything the
// daemon answered - an error of its own included - means it's reachable.
func (c *IncusCommand) NoteError(err error) {
	c.setConnected(!IsConnectionError(err))
}

func (c *IncusCommand) setConnected(connected bool) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	c.connected = connected
}

// GetInstances lists every instance in scope, with its config, state and
// snapshots: one request for what would otherwise be one per instance.
func (c *IncusCommand) GetInstances() ([]*Instance, error) {
	client, project, allProjects := c.scope()
	listing := c.runtimes.beginListing()

	fulls, err := listInstances(client, allProjects)
	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	instances := make([]*Instance, len(fulls))
	for i := range fulls {
		instanceClient := client
		if allProjects {
			instanceClient = client.UseProject(fulls[i].Project)
		}

		instances[i] = c.newInstance(fulls[i], project, instanceClient, listing)
	}

	c.runtimes.prune(func(listed string) bool { return allProjects || listed == project }, instances, listing)

	return instances, nil
}

// newInstance wraps one listed instance. fallbackProject names it for a
// listing that left the instance's own project out.
func (c *IncusCommand) newInstance(full api.InstanceFull, fallbackProject string, client incus.InstanceServer, listing uint64) *Instance {
	project := full.Project
	if project == "" {
		project = fallbackProject
	}

	instance := &Instance{
		Name:      full.Name,
		Project:   project,
		Remote:    c.RemoteName(),
		Instance:  full,
		Client:    client,
		OSCommand: c.OSCommand,
		Log:       c.Log,
		Tr:        c.Tr,
	}

	c.runtimes.attach(instance, listing)

	return instance
}

// scope is the client, its project and whether the panels list every
// project, read together so a project switch can't land between them.
func (c *IncusCommand) scope() (incus.InstanceServer, string, bool) {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	return c.client, c.projectName, c.allProjects
}

// GetImages lists the images stored on the server.
func (c *IncusCommand) GetImages() ([]*Image, error) {
	client := c.Client()

	apiImages, err := c.listImages(client)
	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	// The images stand without their users: a client may be allowed one
	// and not the other.
	users, usersErr := imageUsers(client)
	if usersErr != nil {
		c.Log.Warn(usersErr)
	}

	ownImages := make([]*Image, len(apiImages))

	for i := range apiImages {
		apiImage := apiImages[i]

		ownImages[i] = &Image{
			Fingerprint:  apiImage.Fingerprint,
			Image:        apiImage,
			UsedBy:       users[apiImage.Fingerprint],
			UsersUnknown: usersErr != nil,
			Client:       c.clientFor(apiImage.Project),
			OSCommand:    c.OSCommand,
			Log:          c.Log,
			Tr:           c.Tr,
		}
	}

	return ownImages, nil
}

// imageUsers maps each image's fingerprint to the instances created from it,
// by volatile.base_image. Every project's, whatever the panels are scoped
// to: a project without features.images uses default's images, so an image
// listed in one project can be what another project's instances came from,
// and prune has to know. A client limited to some projects falls back to
// the ones it can see.
func imageUsers(client incus.InstanceServer) (map[string][]string, error) {
	instances, err := client.GetInstancesAllProjects(api.InstanceTypeAny)
	if err != nil {
		instances, err = client.GetInstances(api.InstanceTypeAny)
		if err != nil {
			return nil, err
		}
	}

	users := map[string][]string{}

	for _, instance := range instances {
		if fingerprint := instance.Config["volatile.base_image"]; fingerprint != "" {
			users[fingerprint] = append(users[fingerprint], instance.Project+"/"+instance.Name)
		}
	}

	return users, nil
}

func (c *IncusCommand) listImages(client incus.InstanceServer) ([]api.Image, error) {
	if c.IsAllProjects() {
		return client.GetImagesAllProjects()
	}

	return client.GetImages()
}

// GetProfiles lists the profiles, every project's in the all-projects view.
func (c *IncusCommand) GetProfiles() ([]*Profile, error) {
	client := c.Client()

	var apiProfiles []api.Profile

	var err error

	if c.IsAllProjects() {
		apiProfiles, err = client.GetProfilesAllProjects()
	} else {
		apiProfiles, err = client.GetProfiles()
	}

	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	ownProfiles := make([]*Profile, len(apiProfiles))

	for i := range apiProfiles {
		apiProfile := apiProfiles[i]

		ownProfiles[i] = &Profile{
			Name:      apiProfile.Name,
			Profile:   apiProfile,
			Client:    c.clientFor(apiProfile.Project),
			OSCommand: c.OSCommand,
			Log:       c.Log,
			Tr:        c.Tr,
		}
	}

	return ownProfiles, nil
}

// GetNetworks lists the server's networks, managed and unmanaged alike.
func (c *IncusCommand) GetNetworks() ([]*Network, error) {
	client := c.Client()

	apiNetworks, err := c.listNetworks(client)
	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	ownNetworks := make([]*Network, len(apiNetworks))

	for i := range apiNetworks {
		apiNetwork := apiNetworks[i]

		ownNetworks[i] = &Network{
			Name:    apiNetwork.Name,
			Network: apiNetwork,
			Client:  c.clientFor(apiNetwork.Project),
			ClientFor: func(project string) incus.InstanceServer {
				return c.Client().UseProject(project)
			},
			OSCommand: c.OSCommand,
			Log:       c.Log,
			Tr:        c.Tr,
		}
	}

	return ownNetworks, nil
}

func (c *IncusCommand) listNetworks(client incus.InstanceServer) ([]api.Network, error) {
	if c.IsAllProjects() {
		return client.GetNetworksAllProjects()
	}

	return client.GetNetworks()
}

// GetVolumes lists the volumes of every storage pool. The API is per-pool,
// so this is one request per pool on top of the pool listing; a pool that
// errors is skipped rather than failing the whole list, since one broken
// pool shouldn't empty the panel.
func (c *IncusCommand) GetVolumes() ([]*Volume, error) {
	client := c.Client()

	pools, err := client.GetStoragePools()
	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	ownVolumes := []*Volume{}

	for _, pool := range pools {
		apiVolumes, err := c.listVolumes(client, pool.Name)
		if err != nil {
			c.Log.Warn(err)
			continue
		}

		var space *api.ResourcesStoragePoolSpace
		if resources, err := client.GetStoragePoolResources(pool.Name); err == nil {
			space = &resources.Space
		}

		for i := range apiVolumes {
			apiVolume := apiVolumes[i]

			ownVolumes = append(ownVolumes, &Volume{
				Pool:       pool.Name,
				Name:       apiVolume.Name,
				Volume:     apiVolume,
				PoolDriver: pool.Driver,
				PoolSpace:  space,
				Client:     c.clientFor(apiVolume.Project),
				OSCommand:  c.OSCommand,
				Log:        c.Log,
				Tr:         c.Tr,
			})
		}
	}

	readVolumeDetails(ownVolumes)

	return ownVolumes, nil
}

// requestsInFlight caps a fan-out of one request per item: volumes' usage
// and snapshots, a network's leases per project.
const requestsInFlight = 8

// readVolumeDetails asks after what the listing doesn't carry: each
// volume's usage, and a custom volume's snapshots - an instance's own
// volumes' are the instance's. Requests a volume, several in flight at a
// time. A volume the daemon can't size keeps a nil Usage.
func readVolumeDetails(volumes []*Volume) {
	var wait sync.WaitGroup

	slots := make(chan struct{}, requestsInFlight)

	for _, volume := range volumes {
		wait.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			state, err := volume.Client.GetStoragePoolVolumeState(volume.Pool, volume.Volume.Type, volume.Name)
			if err == nil && state.Usage != nil && state.Usage.Used > 0 {
				volume.Usage = state.Usage
			}

			if volume.IsCustom() {
				if snapshots, err := volume.Client.GetStoragePoolVolumeSnapshots(volume.Pool, volume.Volume.Type, volume.Name); err == nil {
					volume.SnapshotList = snapshots
				}
			}
		})
	}

	wait.Wait()
}

func listInstances(client incus.InstanceServer, allProjects bool) ([]api.InstanceFull, error) {
	if allProjects {
		return client.GetInstancesFullAllProjects(api.InstanceTypeAny)
	}

	return client.GetInstancesFull(api.InstanceTypeAny)
}

func (c *IncusCommand) listVolumes(client incus.InstanceServer, pool string) ([]api.StorageVolume, error) {
	if c.IsAllProjects() {
		return client.GetStoragePoolVolumesAllProjects(pool)
	}

	return client.GetStoragePoolVolumes(pool)
}
