package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
)

func TestParseComposeConfig(t *testing.T) {
	name, services, err := parseComposeConfig(`{
		"name": "playground",
		"services": {
			"web": {"image": "images:alpine/3.20"},
			"db": {"image": "postgres:16", "deploy": {"replicas": 2}}
		}
	}`)

	assert.NoError(t, err)
	assert.Equal(t, "playground", name)

	byName := map[string]commands.ComposeService{}
	for _, service := range services {
		byName[service.Name] = service
	}

	assert.Equal(t, "images:alpine/3.20", byName["web"].Image)
	// Replicas defaults to one, the way compose reads a service with no
	// deploy block.
	assert.Equal(t, 1, byName["web"].Replicas)
	assert.Equal(t, 2, byName["db"].Replicas)
}

func TestParseComposeConfigNoServices(t *testing.T) {
	name, services, err := parseComposeConfig(`{"name": "playground"}`)

	assert.NoError(t, err)
	assert.Equal(t, "playground", name)
	assert.Empty(t, services)
}

func TestParseComposeConfigInvalidJSON(t *testing.T) {
	_, _, err := parseComposeConfig("not json")
	assert.Error(t, err)
}
