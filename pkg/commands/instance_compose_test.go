package commands

import (
	"errors"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubbornServer is a daemon whose instance ignores a clean shutdown, as an
// nginx replica sometimes does: Incus fails the stop at the timeout and
// leaves it running, and only a forced stop takes it down.
type stubbornServer struct {
	incus.InstanceServer

	running bool
	stops   []api.InstanceStatePut
	patches []any
	// calls is every state change and marker write, in order.
	calls []string
}

type doneOperation struct {
	incus.Operation

	err error
}

func (o doneOperation) Wait() error { return o.err }

func (s *stubbornServer) UpdateInstanceState(_ string, put api.InstanceStatePut, _ string) (incus.Operation, error) {
	s.stops = append(s.stops, put)
	s.calls = append(s.calls, put.Action+map[bool]string{true: " --force"}[put.Force])

	if put.Action != "stop" {
		return doneOperation{}, nil
	}

	if !put.Force {
		return doneOperation{err: errors.New(`Failed shutting down instance, status is "Running": context deadline exceeded`)}, nil
	}

	s.running = false

	return doneOperation{}, nil
}

func (s *stubbornServer) GetInstanceState(string) (*api.InstanceState, string, error) {
	code := api.Stopped
	if s.running {
		code = api.Running
	}

	return &api.InstanceState{StatusCode: code}, "", nil
}

func (s *stubbornServer) RawQuery(method string, path string, data any, _ string) (*api.Response, string, error) {
	s.patches = append(s.patches, []any{method, path, data})
	s.calls = append(s.calls, "mark "+data.(map[string]any)["config"].(map[string]string)[healthStoppedKey])

	return &api.Response{}, "", nil
}

func stubbornInstance(server *stubbornServer, config map[string]string) *Instance {
	return &Instance{
		Name:     "web-1",
		Project:  "playground",
		Instance: api.InstanceFull{Instance: api.Instance{ExpandedConfig: config}},
		Client:   server,
		Log:      NewDummyLog(),
	}
}

func TestStoppingAReplicaKillsItWhenItWontShutDown(t *testing.T) {
	server := &stubbornServer{running: true}
	replica := stubbornInstance(server, map[string]string{composeServiceKey: "web"})

	require.NoError(t, replica.Stop())

	assert.False(t, server.running)
	require.Len(t, server.stops, 2)
	assert.Equal(t, composeStopTimeout, server.stops[0].Timeout)
	assert.False(t, server.stops[0].Force)
	assert.True(t, server.stops[1].Force)
	assert.Equal(t, []any{[]any{
		"PATCH", "/1.0/instances/web-1?project=playground",
		map[string]any{"config": map[string]string{healthStoppedKey: "true"}},
	}}, server.patches)
}

func TestStartingAReplicaClearsTheStoppedMarker(t *testing.T) {
	server := &stubbornServer{}
	replica := stubbornInstance(server, map[string]string{composeServiceKey: "web"})

	require.NoError(t, replica.Start())

	assert.Equal(t, []any{[]any{
		"PATCH", "/1.0/instances/web-1?project=playground",
		map[string]any{"config": map[string]string{healthStoppedKey: "false"}},
	}}, server.patches)
}

// Outside compose, a failed clean shutdown is still reported rather than
// escalated, and nothing is written to the instance's config.
func TestStoppingAPlainInstanceDoesNotEscalate(t *testing.T) {
	server := &stubbornServer{running: true}
	instance := stubbornInstance(server, nil)

	require.Error(t, instance.Stop())

	assert.True(t, server.running)
	assert.Len(t, server.stops, 1)
	assert.Empty(t, server.patches)
}

// incus-compose's restart is its whole stop, then its start, so a replica
// that won't shut down is killed rather than left running.
func TestRestartingAReplicaStopsThenStartsIt(t *testing.T) {
	server := &stubbornServer{running: true}
	replica := stubbornInstance(server, map[string]string{composeServiceKey: "web"})

	require.NoError(t, replica.Restart())

	assert.Equal(t, []string{"mark true", "stop", "stop --force", "mark false", "start"}, server.calls)
	assert.Equal(t, composeRestartTimeout, server.stops[0].Timeout)
}

// A frozen replica answers no healthcheck, so the marker goes on before the
// freeze and comes off only once it's thawed.
func TestPausingAReplicaMarksItAroundTheFreeze(t *testing.T) {
	server := &stubbornServer{running: true}
	replica := stubbornInstance(server, map[string]string{composeServiceKey: "web"})

	require.NoError(t, replica.Freeze())
	require.NoError(t, replica.Unfreeze())

	assert.Equal(t, []string{"mark true", "freeze", "unfreeze", "mark false"}, server.calls)
}
