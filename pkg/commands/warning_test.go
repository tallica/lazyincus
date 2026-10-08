package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

func TestWarningEntity(t *testing.T) {
	assert.Equal(t, "incusbr0", (&Warning{Warning: api.Warning{EntityURL: "/1.0/networks/incusbr0?project=default"}}).Entity())
	assert.Empty(t, (&Warning{}).Entity(), "a server-wide warning")
}

// a acknowledges a new warning and puts an acknowledged one back: the two
// statuses a client may set.
func TestAcknowledgeWarningToggles(t *testing.T) {
	server := incustest.New(incustest.Server{})
	server.SetWarnings([]api.Warning{{UUID: "w1", WarningPut: api.WarningPut{Status: WarningNew}}})
	command := newEventsCommand(server)

	status := func() string {
		warnings, err := command.GetWarnings()
		require.NoError(t, err)
		require.Len(t, warnings, 1)

		return warnings[0].Warning.Status
	}

	warnings, err := command.GetWarnings()
	require.NoError(t, err)

	require.NoError(t, command.AcknowledgeWarning(warnings[0]))
	assert.Equal(t, WarningAcknowledged, status())

	warnings, _ = command.GetWarnings()
	require.NoError(t, command.AcknowledgeWarning(warnings[0]))
	assert.Equal(t, WarningNew, status())
}
