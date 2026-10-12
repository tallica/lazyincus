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

// A config command str.ToArgv can't read - a quote left open, a backslash to
// end on - is an error to report, not the panic that used to take the app down
// with it, and a line of nothing is nothing to run.
func TestExecutableFromStringRefusesALineItCantRead(t *testing.T) {
	c := NewDummyOSCommand()

	for _, line := range []string{`sh -c "echo hi`, `sh -c 'echo hi`, `sh -c echo hi\`, "   ", ""} {
		cmd, err := c.ExecutableFromString(line)
		assert.Error(t, err, line)

		// Not assert.Nil: failing on it prints the command, environment and all.
		if cmd != nil {
			t.Errorf("%q built a command anyway", line)
		}
	}
}

// Quote escapes for this parser, so what it wraps is one argument again: the
// two belong together, and a line going through anything else would split it.
func TestQuoteRoundTripsThroughExecutableFromString(t *testing.T) {
	c := NewDummyOSCommand()

	for _, value := range []string{`a b`, `a"b`, `a\b`, `a$b`, "a`b"} {
		cmd, err := c.ExecutableFromString("vi " + c.Quote(value))
		assert.NoError(t, err, value)
		assert.Equal(t, []string{"vi", value}, cmd.Args, value)
	}
}
