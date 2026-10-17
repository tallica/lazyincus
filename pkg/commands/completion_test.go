package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// As incus 7.4 answers `__complete --project default st` and `start s`.
func TestParseCobraCompletion(t *testing.T) {
	verbs := "start\tStart instances\nstop\tStop instances\nstorage\tManage storage pools and volumes\n:4\n"
	assert.Equal(t, Completion{Candidates: []string{"start", "stop", "storage"}}, parseCobraCompletion(verbs, "st"))

	names := "bsz-web\nlocal:\nsite-a:\n:6\n"
	assert.Equal(t, Completion{Candidates: []string{"site-a:"}, NoSpace: true}, parseCobraCompletion(names, "s"))

	assert.Equal(t, Completion{}, parseCobraCompletion("start\n:1\n", ""))
	assert.Equal(t, Completion{}, parseCobraCompletion("no directive\n", ""))
}
