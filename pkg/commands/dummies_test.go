package commands

import (
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
