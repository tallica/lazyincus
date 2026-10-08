package commands

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAFailedCommandSaysWhy(t *testing.T) {
	c := NewDummyOSCommand()

	_, err := c.RunExecutableWithOutput(exec.Command("sh", "-c", "echo nope >&2; exit 1"))
	assert.EqualError(t, err, "nope", "stderr went into the combined output")

	_, err = sanitisedCommandOutput(exec.Command("sh", "-c", "echo out; echo nope >&2; exit 1").Output())
	assert.EqualError(t, err, "nope\n", "stderr kept apart")
}
