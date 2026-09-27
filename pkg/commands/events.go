package commands

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
)

// Event is one entry of the daemon's event stream, reduced to what the
// panels act on.
type Event struct {
	Type    string
	Project string
	// Action is a lifecycle event's action, "instance-started", or an
	// operation's description, "Restarting instance".
	Action string
	// Operation, Status and Instances are an operation's: its ID, where
	// it's got to, and the instances it acts on.
	Operation string
	Status    api.StatusCode
	Instances []string
}

// EventListener is the part of *incus.EventListener lazyincus uses, so a
// stand-in daemon can hand out one of its own. An alias of a literal, so
// incustest can spell the same type without importing this package, whose
// tests import it.
type EventListener = interface {
	AddHandler(types []string, function func(api.Event)) (*incus.EventTarget, error)
	Wait() error
	Disconnect()
}

// eventSource is a client that hands out its own EventListener - incustest's.
type eventSource interface {
	Listen(allProjects bool, types []string) (EventListener, error)
}

func listen(client incus.InstanceServer, allProjects bool, types []string) (EventListener, error) {
	if source, ok := client.(eventSource); ok {
		return source.Listen(allProjects, types)
	}

	if allProjects {
		return client.GetEventsAllProjectsByType(types)
	}

	return client.GetEventsByType(types)
}

// ListenForEvents passes the daemon's events of the given types, for the
// scope the panels list, to handle until ctx is done or the connection
// drops. It returns why it stopped: nil for ctx, the drop otherwise.
func (c *IncusCommand) ListenForEvents(ctx context.Context, types []string, handle func(Event)) error {
	client, project, allProjects := c.scope()

	listener, err := listen(client, allProjects, types)
	if err != nil {
		return err
	}

	if _, err := listener.AddHandler(types, func(event api.Event) {
		if parsed, ok := parseEvent(event, project); ok {
			handle(parsed)
		}
	}); err != nil {
		listener.Disconnect()
		return err
	}

	dropped := make(chan error, 1)
	go func() { dropped <- listener.Wait() }()

	select {
	case <-ctx.Done():
		listener.Disconnect()
		return nil
	case err := <-dropped:
		return err
	}
}

// parseEvent reads the event's metadata. fallbackProject names the project
// of an event from a daemon too old to say.
func parseEvent(event api.Event, fallbackProject string) (Event, bool) {
	parsed := Event{Type: event.Type, Project: event.Project}
	if parsed.Project == "" {
		parsed.Project = fallbackProject
	}

	switch event.Type {
	case api.EventTypeLifecycle:
		var lifecycle api.EventLifecycle
		if json.Unmarshal(event.Metadata, &lifecycle) != nil {
			return Event{}, false
		}

		parsed.Action = lifecycle.Action
	case api.EventTypeOperation:
		var operation api.Operation
		if json.Unmarshal(event.Metadata, &operation) != nil {
			return Event{}, false
		}

		parsed.Action = operation.Description
		parsed.Operation = operation.ID
		parsed.Status = operation.StatusCode

		for _, path := range operation.Resources["instances"] {
			if name, ok := instanceName(path); ok {
				parsed.Instances = append(parsed.Instances, name)
			}
		}
	default:
		return Event{}, false
	}

	return parsed, true
}

// instanceName reads the instance out of "/1.0/instances/web?project=x",
// leaving out a path to anything below an instance.
func instanceName(path string) (string, bool) {
	path, _, _ = strings.Cut(path, "?")

	name, ok := strings.CutPrefix(path, "/1.0/instances/")
	if !ok || name == "" || strings.Contains(name, "/") {
		return "", false
	}

	name, err := url.PathUnescape(name)

	return name, err == nil
}
