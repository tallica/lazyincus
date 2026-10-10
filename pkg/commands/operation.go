package commands

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lxc/incus/v7/shared/api"
)

// Operation is a daemon operation: a task, or a websocket an exec or a
// console holds open.
type Operation struct {
	Operation api.Operation
	// Project is the operation's, which api.Operation doesn't carry: the
	// event says, and a listing's resource URLs do.
	Project string
	// Ended is when the event saying it had ended was sent, by the daemon's
	// clock: the daemon moves UpdatedAt for new metadata, not a new status.
	Ended time.Time
}

// operationClassToken is a join or certificate token: running for as long as
// the token is valid, with the secret in its metadata.
const operationClassToken = "token"

// IsToken reports an operation the Operations tab leaves out.
func (o *Operation) IsToken() bool {
	return strings.EqualFold(o.Operation.Class, operationClassToken)
}

func (o *Operation) Key() string {
	return o.Operation.ID
}

// IsFinal reports an operation that has ended, however.
func (o *Operation) IsFinal() bool {
	return o.Operation.StatusCode.IsFinal()
}

// Status is the status in lower case, as the panels spell statuses.
func (o *Operation) Status() string {
	return strings.ToLower(o.Operation.StatusCode.String())
}

// Took is how long an ended operation ran, zero for one still running. One
// only ever listed falls back on its last update.
func (o *Operation) Took() time.Duration {
	if !o.IsFinal() {
		return 0
	}

	end := o.Ended
	if end.IsZero() {
		end = o.Operation.UpdatedAt
	}

	return max(end.Sub(o.Operation.CreatedAt), 0)
}

// Progress is where a long operation has got to, as the daemon words it:
// an image download's "rootfs: 23% (5.20MB/s)", say. Every kind of progress
// is a metadata key of its own ending in `_progress`.
func (o *Operation) Progress() string {
	keys := make([]string, 0, len(o.Operation.Metadata))
	for key := range o.Operation.Metadata {
		if strings.HasSuffix(key, "_progress") {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	for _, key := range keys {
		if progress, ok := o.Operation.Metadata[key].(string); ok && progress != "" {
			return progress
		}
	}

	return ""
}

// Resources names what the operation acts on, kind first, sorted:
// "instances web".
func (o *Operation) Resources() []string {
	var resources []string

	for kind, paths := range o.Operation.Resources {
		for _, path := range paths {
			resources = append(resources, kind+" "+resourceName(path))
		}
	}

	sort.Strings(resources)

	return resources
}

// Target is the first thing the operation acts on, by name alone, for its
// row: an instance's operations name its snapshots too, and the instance is
// what the row wants.
func (o *Operation) Target() string {
	for _, kind := range []string{"instances", "images", "storage_volumes", "networks", "profiles"} {
		if paths := o.Operation.Resources[kind]; len(paths) > 0 {
			return lastSegment(resourceName(paths[0]))
		}
	}

	return ""
}

// resourceName is a resource URL less its API prefix and query:
// "/1.0/instances/web?project=x" is "web".
func resourceName(path string) string {
	path, _, _ = strings.Cut(path, "?")
	path = strings.TrimPrefix(path, "/1.0/")

	if _, rest, ok := strings.Cut(path, "/"); ok {
		path = rest
	}

	if unescaped, err := url.PathUnescape(path); err == nil {
		return unescaped
	}

	return path
}

func lastSegment(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

// operationProject reads the project out of the first resource URL that
// names one, fallback for none.
func operationProject(operation api.Operation, fallback string) string {
	for _, paths := range operation.Resources {
		for _, path := range paths {
			_, query, ok := strings.Cut(path, "?")
			if !ok {
				continue
			}

			values, err := url.ParseQuery(query)
			if err == nil && values.Get("project") != "" {
				return values.Get("project")
			}
		}
	}

	return fallback
}

// GetOperations lists the operations the daemon holds for the panels'
// scope: those running, and those that ended within the last 5s.
func (c *IncusCommand) GetOperations() ([]*Operation, error) {
	client, project, allProjects := c.scope()

	var (
		listed []api.Operation
		err    error
	)

	if allProjects {
		listed, err = client.GetOperationsAllProjects()
	} else {
		listed, err = client.GetOperations()
	}

	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	if project == "" {
		project = api.ProjectDefaultName
	}

	operations := make([]*Operation, 0, len(listed))

	for _, operation := range listed {
		listed := &Operation{Operation: operation, Project: operationProject(operation, project)}
		if !listed.IsToken() {
			operations = append(operations, listed)
		}
	}

	return operations, nil
}

// CancelOperation asks the daemon to cancel the operation, which it does
// only for one that says it may.
func (c *IncusCommand) CancelOperation(operation *Operation) error {
	return c.Client().UseProject(operation.Project).DeleteOperation(operation.Operation.ID)
}
