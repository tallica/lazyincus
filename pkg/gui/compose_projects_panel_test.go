package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseComposeProjectName(t *testing.T) {
	name, err := parseComposeProjectName(`{"name": "playground", "services": {}}`)
	assert.NoError(t, err)
	assert.Equal(t, "playground", name)
}

func TestParseComposeProjectNameInvalidJSON(t *testing.T) {
	_, err := parseComposeProjectName("not json")
	assert.Error(t, err)
}
