package commands

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/cliconfig"
	"github.com/stretchr/testify/assert"
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
		"lenny":  {Protocol: "incus"},
		"images": {Protocol: "simplestreams", Public: true},
		"docker": {Protocol: "oci", Public: true},
	}}}

	assert.Equal(t, []string{"lenny", "local"}, command.InstanceRemoteNames())
}
