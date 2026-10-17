package commands

import (
	"slices"
	"strconv"
	"strings"
)

// Completion is what a command line can go on with at the word being typed.
type Completion struct {
	Candidates []string

	// NoSpace is a candidate that goes on rather than ending the word: a
	// remote's `name:`, before the instance on it.
	NoSpace bool
}

// cobra's ShellCompDirective bits that change what's offered.
const (
	cobraDirectiveError   = 1
	cobraDirectiveNoSpace = 2
)

// CompleteIncus asks the incus CLI, through cobra's hidden __complete, what
// can follow args at word on remote: verbs, flags, and names the CLI reads off
// the daemon itself.
func (c *OSCommand) CompleteIncus(remote string, args []string, word string) (Completion, error) {
	cmd := WithRemote(c.NewCmd("incus", slices.Concat([]string{"__complete"}, args, []string{word})...), remote)

	// stdout only: stderr carries cobra's "Completion ended with directive".
	output, err := cmd.Output()
	if err != nil {
		return Completion{}, WrapError(err)
	}

	return parseCobraCompletion(string(output), word), nil
}

// parseCobraCompletion reads __complete's output: a candidate a line, a tab
// before any description, and a last line of `:` and the directive. cobra
// leaves a completion function's answer unfiltered, so word filters it here.
func parseCobraCompletion(output, word string) Completion {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[len(lines)-1], ":") {
		return Completion{}
	}

	directive, err := strconv.Atoi(strings.TrimPrefix(lines[len(lines)-1], ":"))
	if err != nil || directive&cobraDirectiveError != 0 {
		return Completion{}
	}

	completion := Completion{NoSpace: directive&cobraDirectiveNoSpace != 0}

	for _, line := range lines[:len(lines)-1] {
		candidate, _, _ := strings.Cut(line, "\t")
		if candidate != "" && strings.HasPrefix(candidate, word) {
			completion.Candidates = append(completion.Candidates, candidate)
		}
	}

	return completion
}

// ComposeVerbs are incus-compose's commands, read off `incus-compose --help`
// at 1.3.4: it has no completion of its own to ask.
var ComposeVerbs = []string{
	"up", "down", "build", "start", "stop", "kill", "restart", "run", "pause", "unpause",
	"ps", "pull", "config", "exec", "cp", "top", "events", "port", "logs",
	"backup", "list", "port-forward", "incus", "healthd", "version",
}
