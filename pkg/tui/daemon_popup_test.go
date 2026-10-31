package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
)

// recordOperations: ended operations a second apart, described as given.
func recordOperations(m *Model, descriptions ...string) {
	for i, description := range descriptions {
		operation := testOperation(fmt.Sprintf("op%02d", i), api.Success, 0)
		operation.Operation.Description = description
		operation.Operation.CreatedAt = operationStart.Add(time.Duration(i) * time.Second)
		m.recordOperation(operation)
	}
}

func warningsServer() *incustest.Server {
	server := eventServer()
	server.SetWarnings([]api.Warning{
		{
			UUID: "kvm", WarningPut: api.WarningPut{Status: "new"}, Type: "Instance type not operational",
			LastMessage: "KVM support is missing (no /dev/kvm)", Severity: "low", Count: 19,
		},
		{
			UUID: "disk", WarningPut: api.WarningPut{Status: "new"}, Type: "Storage pool almost full",
			Severity: "high", Count: 2,
		},
	})

	return server
}

func popupText(m *Model) string {
	return strings.Join(screen(m), "\n")
}

func TestWOpensOnNewWarningsThenTheLastList(t *testing.T) {
	m := start(t, options{server: warningsServer()}, 140, 40)

	press(m, "W")
	assert.Contains(t, popupText(m), "Operations - Warnings")
	assert.Contains(t, popupText(m), "a: acknowledge, d: delete, enter: details")
	assert.Contains(t, popupText(m), "Storage pool almost full")
	assert.Contains(t, footer(m), "[ ]: switch list")

	press(m, "[")
	assert.Contains(t, popupText(m), "d: cancel, enter: details")
	assert.Contains(t, popupText(m), "No operations")
	press(m, "esc")
	assert.Empty(t, m.popups)

	press(m, "W")
	assert.Contains(t, popupText(m), "d: cancel, enter: details")
	press(m, "]")
	assert.Contains(t, popupText(m), "a: acknowledge, d: delete")
	press(m, "]")
	assert.Contains(t, popupText(m), "d: cancel, enter: details")
	require.Len(t, m.popups, 1, "switching replaces the popup")
}

func TestFCutsTheOperationsDownByStatus(t *testing.T) {
	m := start(t, options{}, 140, 40)
	recordOperations(m, "Starting instance")
	failed := testOperation("failed", api.Failure, 0)
	failed.Operation.Description = "Stopping instance"
	m.recordOperation(failed)

	press(m, "W", "f")
	text := popupText(m)
	assert.Contains(t, text, "Operations (failed) - Warnings")
	assert.Contains(t, text, "✗ failure")
	assert.NotContains(t, text, "Starting instance")

	press(m, "f")
	text = popupText(m)
	assert.Contains(t, text, "Operations (succeeded) - Warnings")
	assert.Contains(t, text, "Starting instance")
	assert.NotContains(t, text, "Stopping instance")

	press(m, "f")
	text = popupText(m)
	assert.Contains(t, text, "Operations - Warnings")
	assert.Contains(t, text, "Stopping instance")
}

// The fuzzy filter would put the exact match first; the operations stay in time order.
func TestFilteringTheOperationsKeepsTheirOrder(t *testing.T) {
	m := start(t, options{}, 140, 40)
	recordOperations(m, "Deleting snapshot", "Snapshotting instance", "Starting instance")

	press(m, "W", "/")
	typeText(m, "snapshot")
	text := popupText(m)
	assert.Contains(t, footer(m), "filter: snapshot")
	assert.NotContains(t, text, "Starting instance")
	assert.Regexp(t, regexp.MustCompile(`Snapshotting instance[^\n]*\n[^\n]*Deleting snapshot`), text)

	// esc leaves the filter, then the popup.
	press(m, "enter", "esc")
	assert.Contains(t, popupText(m), "Starting instance")
	assert.True(t, m.daemonOpen(daemonOperations))
	press(m, "esc")
	assert.Empty(t, m.popups)
}

// Left with esc or enter, the prompt isn't what closing the popup goes back to.
func TestClosingWAfterFilteringLeavesThePrompt(t *testing.T) {
	m := start(t, options{}, 140, 40)
	recordOperations(m, "Starting instance")

	// enter keeps the filter, which the first esc on the list clears.
	for leave, escs := range map[string]int{"esc": 1, "enter": 2} {
		press(m, "W", "/")
		typeText(m, "zzz")
		assert.Contains(t, popupText(m), "no matches")
		assert.Equal(t, filterView, m.currentView())

		press(m, leave)
		assert.Equal(t, menuView, m.currentView())

		if escs == 2 {
			press(m, "esc")
			assert.Contains(t, popupText(m), "Starting instance")
		}

		press(m, "esc")
		assert.Empty(t, m.popups)
		assert.NotEqual(t, filterView, m.currentView())
		assert.False(t, m.filter.active)
	}
}

// On the first row the cursor follows the newest; further down, the rows on screen stay put.
func TestOperationsStayPutAsNewOnesArrive(t *testing.T) {
	m := start(t, options{}, 140, 30)
	recordOperations(m, slices.Repeat([]string{"Executing command"}, 40)...)
	arrive := func(id string, at time.Duration) {
		operation := testOperation(id, api.Success, 0)
		operation.Operation.CreatedAt = operationStart.Add(at)
		m.recordOperation(operation)
		m.fitViews()
	}
	list := func() *panel { return m.daemonPopup().menu.list }
	selected := func() string { return m.daemonPopup().menu.selectedItem().key }

	press(m, "W")
	arrive("newest", time.Hour)
	assert.Equal(t, "newest", selected())

	press(m, "down", "down", "down", "down", "down")
	assert.Equal(t, []int{5, 0}, []int{list().selected, list().origin})
	reading := selected()

	arrive("newer still", 2*time.Hour)
	assert.Equal(t, []int{6, 1}, []int{list().selected, list().origin})
	assert.Equal(t, reading, selected())
}

// Looked at, the operations' failures stop being news.
func TestOpeningTheOperationsSeesTheirFailures(t *testing.T) {
	m := start(t, options{server: eventServer()}, 140, 16)
	m.recordOperation(&commands.Operation{Operation: liveOperation("failed", api.Failure), Project: "default"})
	assert.Contains(t, footer(m), "1 failed")

	press(m, "W")
	press(m, "esc")
	assert.NotContains(t, footer(m), "failed")
}

func TestTheProjectColumnShowsOnlyWhenTheLogSpansProjects(t *testing.T) {
	m := start(t, options{}, 140, 40)
	recordOperations(m, "Starting instance")
	press(m, "W")
	rows := func() string { return strings.Join(m.daemonPopup().menu.list.rows, "\n") }
	assert.NotContains(t, rows(), "default")

	other := testOperation("other", api.Success, 0)
	other.Project = "prod"
	m.recordOperation(other)
	m.fitViews()
	assert.Contains(t, rows(), "prod")
	assert.Contains(t, rows(), "default")
}
