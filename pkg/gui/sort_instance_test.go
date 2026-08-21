package gui

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func instanceWithStatus(name, status string) *commands.Instance {
	return &commands.Instance{
		Name:     name,
		Instance: api.Instance{InstancePut: api.InstancePut{}, Name: name, Status: status},
	}
}

func TestSortInstances(t *testing.T) {
	scenarios := []struct {
		name     string
		a        *commands.Instance
		b        *commands.Instance
		expected bool
	}{
		{
			name:     "running before stopped",
			a:        instanceWithStatus("b", "Running"),
			b:        instanceWithStatus("a", "Stopped"),
			expected: true,
		},
		{
			name:     "stopped after frozen",
			a:        instanceWithStatus("a", "Stopped"),
			b:        instanceWithStatus("b", "Frozen"),
			expected: false,
		},
		{
			name:     "same status sorts by name",
			a:        instanceWithStatus("a", "Running"),
			b:        instanceWithStatus("b", "Running"),
			expected: true,
		},
		{
			name:     "same status reverse name",
			a:        instanceWithStatus("b", "Running"),
			b:        instanceWithStatus("a", "Running"),
			expected: false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			assert.Equal(t, s.expected, sortInstances(s.a, s.b))
		})
	}
}
