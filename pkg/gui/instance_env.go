package gui

import (
	"sort"
	"strings"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// renderInstanceEnv shows the instance's environment variables, set via
// Incus's `environment.*` config keys (e.g. `incus config set <name>
// environment.FOO=bar`) - see doc/instance-exec.md. Read from
// ExpandedConfig so profile-inherited variables show up too, not just
// ones set directly on the instance.
func (gui *Gui) renderInstanceEnv(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceEnvStr(instance) })
}

func (gui *Gui) instanceEnvStr(instance *commands.Instance) string {
	full, ok := instance.Full()
	if !ok {
		return gui.Tr.WaitingForInstanceInfo
	}

	const prefix = "environment."

	vars := []string{}
	for key, value := range full.ExpandedConfig {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		vars = append(vars, strings.TrimPrefix(key, prefix)+"="+value)
	}

	if len(vars) == 0 {
		return gui.Tr.NothingToDisplay
	}

	sort.Strings(vars)

	return strings.Join(vars, "\n")
}
