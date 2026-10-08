package commands

import (
	"strings"

	"github.com/lxc/incus/v7/shared/api"
)

// Warning is a daemon warning: something it noticed and kept noticing, its
// Count how often. Status is the daemon's own "new", "acknowledged" or
// "resolved", shared with every other client.
type Warning struct {
	Warning api.Warning
}

const (
	WarningNew          = "new"
	WarningAcknowledged = "acknowledged"
	WarningResolved     = "resolved"
)

func (w *Warning) Key() string {
	return w.Warning.UUID
}

func (w *Warning) IsNew() bool {
	return strings.EqualFold(w.Warning.Status, WarningNew)
}

// Entity is what the warning is about, from its URL: "networks/incusbr0".
// Empty for one about the server, or whose entity has since gone.
func (w *Warning) Entity() string {
	if w.Warning.EntityURL == "" {
		return ""
	}

	return resourceName(w.Warning.EntityURL)
}

// GetWarnings lists every warning, whatever project the panels are scoped
// to: one about the server belongs to no project, and concerns them all.
func (c *IncusCommand) GetWarnings() ([]*Warning, error) {
	listed, err := c.unscopedClient().GetWarnings()
	c.NoteError(err)

	if err != nil {
		return nil, err
	}

	warnings := make([]*Warning, len(listed))
	for i := range listed {
		warnings[i] = &Warning{Warning: listed[i]}
	}

	return warnings, nil
}

// AcknowledgeWarning acknowledges a new warning, or puts an acknowledged
// one back to new: the two statuses a client may set.
func (c *IncusCommand) AcknowledgeWarning(warning *Warning) error {
	status := WarningAcknowledged
	if !warning.IsNew() {
		status = WarningNew
	}

	return c.unscopedClient().UpdateWarning(warning.Key(), api.WarningPut{Status: status}, "")
}

// DeleteWarning removes the warning; if its cause persists, the daemon
// raises it again.
func (c *IncusCommand) DeleteWarning(warning *Warning) error {
	return c.unscopedClient().DeleteWarning(warning.Key())
}
