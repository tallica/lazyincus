package commands

import (
	"errors"
	"fmt"
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

	status api.StatusCode
	// failAll refuses every state change, forced or not.
	failAll bool
	// stopsLate finishes the stop by itself just as the forced stop comes,
	// which Incus then refuses.
	stopsLate bool
	// calls is every state change and marker write, in order.
	calls []string
	// patch is the last marker write's method, path and body.
	patch []any
}

type doneOperation struct {
	incus.Operation

	err error
}

func (o doneOperation) Wait() error { return o.err }

func (s *stubbornServer) UpdateInstanceState(_ string, put api.InstanceStatePut, _ string) (incus.Operation, error) {
	switch {
	case put.Force:
		s.calls = append(s.calls, put.Action+" --force")
	case put.Timeout > 0:
		s.calls = append(s.calls, fmt.Sprintf("%s %ds", put.Action, put.Timeout))
	default:
		s.calls = append(s.calls, put.Action)
	}

	switch {
	case s.failAll:
		return doneOperation{err: errors.New("refused")}, nil
	case put.Action != "stop":
		return doneOperation{}, nil
	case !put.Force:
		return doneOperation{err: errors.New(`Failed shutting down instance, status is "Running": context deadline exceeded`)}, nil
	case s.stopsLate:
		s.status = api.Stopped

		return doneOperation{err: errors.New("The instance is already stopped")}, nil
	}

	s.status = api.Stopped

	return doneOperation{}, nil
}

func (s *stubbornServer) GetInstanceState(string) (*api.InstanceState, string, error) {
	return &api.InstanceState{StatusCode: s.status}, "", nil
}

func (s *stubbornServer) RawQuery(method string, path string, data any, _ string) (*api.Response, string, error) {
	s.patch = []any{method, path, data}
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

func stubbornReplica(status api.StatusCode) (*stubbornServer, *Instance) {
	server := &stubbornServer{status: status}

	return server, stubbornInstance(server, map[string]string{composeServiceKey: "web"})
}

// A clean stop that doesn't end Stopped is forced: one that timed out,
// and one Incus refuses outright because the replica is in Error.
func TestStoppingAReplicaForcesItWhenItWontShutDown(t *testing.T) {
	for _, status := range []api.StatusCode{api.Running, api.Error} {
		t.Run(status.String(), func(t *testing.T) {
			server, replica := stubbornReplica(status)

			require.NoError(t, replica.Stop())

			assert.Equal(t, api.Stopped, server.status)
			assert.Equal(t, []string{"mark true", "stop 10s", "stop --force"}, server.calls)
		})
	}
}

func TestStartingAReplicaClearsTheStoppedMarker(t *testing.T) {
	server, replica := stubbornReplica(api.Stopped)

	require.NoError(t, replica.Start())

	assert.Equal(t, []string{"mark false", "start"}, server.calls)
	assert.Equal(t, []any{
		"PATCH", "/1.0/instances/web-1?project=playground",
		map[string]any{"config": map[string]string{healthStoppedKey: "false"}},
	}, server.patch)
}

// Outside compose, a failed clean shutdown is reported, not escalated.
func TestStoppingAPlainInstanceDoesNotEscalate(t *testing.T) {
	server := &stubbornServer{status: api.Running}
	instance := stubbornInstance(server, nil)

	require.Error(t, instance.Stop())

	assert.Equal(t, api.Running, server.status)
	assert.Equal(t, []string{"stop 30s"}, server.calls)
}

func TestRestartingAReplicaStopsThenStartsIt(t *testing.T) {
	server, replica := stubbornReplica(api.Running)

	require.NoError(t, replica.Restart())

	assert.Equal(t, []string{"mark true", "stop 60s", "stop --force", "mark false", "start"}, server.calls)
}

// A frozen replica answers no healthcheck, so the marker goes on before the
// freeze and comes off only once it's thawed.
func TestPausingAReplicaMarksItAroundTheFreeze(t *testing.T) {
	server, replica := stubbornReplica(api.Running)

	require.NoError(t, replica.Freeze())
	require.NoError(t, replica.Unfreeze())

	assert.Equal(t, []string{"mark true", "freeze", "unfreeze", "mark false"}, server.calls)
}

// A replica that couldn't be taken down is still running, and mustn't keep
// the mark that tells ic-healthd to leave it alone.
func TestAFailedStopOrFreezeTakesTheMarkOff(t *testing.T) {
	for name, act := range map[string]func(*Instance) error{
		"stop":   (*Instance).Stop,
		"kill":   (*Instance).ForceStop,
		"freeze": (*Instance).Freeze,
	} {
		t.Run(name, func(t *testing.T) {
			server, replica := stubbornReplica(api.Running)
			server.failAll = true

			require.Error(t, act(replica))

			assert.Equal(t, "mark true", server.calls[0])
			assert.Equal(t, "mark false", server.calls[len(server.calls)-1])
		})
	}
}

// The clean stop timed out, then the replica stopped by itself before the
// forced one: it's down, as asked, and keeps the mark.
func TestAReplicaThatStopsLateKeepsTheMark(t *testing.T) {
	server, replica := stubbornReplica(api.Running)
	server.stopsLate = true

	require.NoError(t, replica.Stop())

	assert.Equal(t, api.Stopped, server.status)
	assert.Equal(t, []string{"mark true", "stop 10s", "stop --force"}, server.calls)
}

// A paused replica that can't be stopped is still paused, which the mark
// is there for.
func TestAFailedStopOnAPausedReplicaKeepsTheMark(t *testing.T) {
	server, replica := stubbornReplica(api.Frozen)
	server.failAll = true

	require.Error(t, replica.Stop())

	assert.Equal(t, []string{"mark true", "stop 10s", "stop --force"}, server.calls)
}
