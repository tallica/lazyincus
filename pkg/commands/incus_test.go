package commands

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

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
