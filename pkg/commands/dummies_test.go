package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDummyIncusCommand(t *testing.T) {
	c := NewDummyIncusCommand()
	assert.NotNil(t, c)
	assert.NotNil(t, c.Log)
	assert.NotNil(t, c.OSCommand)
	assert.NotNil(t, c.Tr)
	assert.Equal(t, "lazyincus", c.Config.Name)
}

func TestInstanceIsVM(t *testing.T) {
	container := &Instance{Name: "web"}
	assert.False(t, container.IsVM())

	container.Instance.Type = "virtual-machine"
	assert.True(t, container.IsVM())
}

func TestInstanceIsRunning(t *testing.T) {
	inst := &Instance{Name: "web"}
	assert.False(t, inst.IsRunning())

	inst.Instance.Status = "Running"
	assert.True(t, inst.IsRunning())

	inst.Instance.Status = "running"
	assert.True(t, inst.IsRunning())

	inst.Instance.Status = "Stopped"
	assert.False(t, inst.IsRunning())
}

func TestAsDeleteError(t *testing.T) {
	assert.NoError(t, asDeleteError(nil))

	// The daemon's exact refusal, as returned by instanceDelete.
	assert.ErrorIs(t, asDeleteError(errors.New("Instance is running")), ErrInstanceRunning)

	other := errors.New("Instance not found")
	assert.ErrorIs(t, asDeleteError(other), other)
}
