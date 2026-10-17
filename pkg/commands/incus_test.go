package commands

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/cliconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both dialers, since the client sets DialContext for a unix socket and
// DialTLSContext for a TLS remote.
func TestCapTransportDialBoundsBothDialers(t *testing.T) {
	var deadlines []time.Time

	record := func(ctx context.Context, _, _ string) (net.Conn, error) {
		deadline, _ := ctx.Deadline()
		deadlines = append(deadlines, deadline)

		return nil, errors.New("not dialing anything")
	}

	transport := &http.Transport{DialContext: record, DialTLSContext: record}

	capTransportDial(transport)

	_, err := transport.DialContext(context.Background(), "tcp", "192.0.2.1:8443")
	assert.Error(t, err)
	_, err = transport.DialTLSContext(context.Background(), "tcp", "192.0.2.1:8443")
	assert.Error(t, err)

	assert.Len(t, deadlines, 2)
	for _, deadline := range deadlines {
		assert.WithinDuration(t, time.Now().Add(dialTimeout), deadline, time.Second)
	}
}

func TestCapTransportDialLeavesAbsentDialersAlone(t *testing.T) {
	transport := &http.Transport{}
	capTransportDial(transport)
	assert.Nil(t, transport.DialContext)
	assert.Nil(t, transport.DialTLSContext)
}

// A published port is reached at the remote's own host, which a unix socket
// or a loopback tunnel to the API doesn't give.
func TestPublishHost(t *testing.T) {
	for remote, want := range map[string]string{
		"https://192.0.2.5:8443":            "192.0.2.5",
		"https://incus.example:8443":        "incus.example",
		"https://[2001:db8::5]:8443":        "2001:db8::5",
		"https://127.0.0.1:8443":            "",
		"https://localhost:8443":            "",
		"https://[::1]:8443":                "",
		"unix://":                           "",
		"unix:///var/lib/incus/unix.socket": "",
		"":                                  "",
	} {
		assert.Equal(t, want, publishHost(remote), remote)
	}
}

// Every remote with instances, by name: `local` too, whatever the OS.
func TestInstanceRemoteNames(t *testing.T) {
	command := &IncusCommand{cliCfg: &cliconfig.Config{Remotes: map[string]cliconfig.Remote{
		"local":  {Protocol: "incus"},
		"web01":  {Protocol: "incus"},
		"images": {Protocol: "simplestreams", Public: true},
		"docker": {Protocol: "oci", Public: true},
	}}}

	assert.Equal(t, []string{"local", "web01"}, command.InstanceRemoteNames())
}

// Stacks on several remotes connect side by side, and GetInstanceServer
// caches each remote's TLS details in the config's Remotes map.
func TestConnectRemoteConcurrently(t *testing.T) {
	cliCfg := &cliconfig.Config{ConfigDir: t.TempDir(), Remotes: map[string]cliconfig.Remote{}}
	names := []string{"a", "b", "c", "d"}

	for _, name := range names {
		cliCfg.Remotes[name] = cliconfig.Remote{Addrs: []string{"https://127.0.0.1:1"}, Protocol: "incus"}
	}

	var wg sync.WaitGroup

	for _, name := range names {
		wg.Go(func() {
			_, err := connectRemote(cliCfg, name)
			assert.Error(t, err)
		})
	}

	wg.Wait()
}

// Every listener's connect writes the client it's on, before it dials, and
// UseProject reads that without a lock: the stream reconnecting while the
// panels make project clients is a race the detector sees, on the real
// client - incustest's has nothing to race on. A port nothing listens on
// keeps the stream failing, and so connecting, for as long as the test runs.
func TestEventStreamDoesntRaceProjectClients(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	client, err := incus.ConnectIncus("https://"+addr, &incus.ConnectionArgs{SkipGetServer: true, InsecureSkipVerify: true})
	require.NoError(t, err)

	command := NewIncusCommandWithClient(nil, nil, nil, nil, client, "closed")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var wait sync.WaitGroup
	wait.Add(2)

	go func() {
		defer wait.Done()

		for ctx.Err() == nil {
			_ = command.ListenForEvents(ctx, []string{"lifecycle"}, func() {}, func(Event) {})
		}
	}()

	go func() {
		defer wait.Done()

		for ctx.Err() == nil {
			command.clientIn("shop")
			command.clientFor("shop")
			command.UseProject("default")
			command.UseAllProjects()
		}
	}()

	wait.Wait()
}
