package gui

import (
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// renderInstanceEnv shows Incus's `environment.*` config keys, read from
// ExpandedConfig so profile-inherited variables show up too.
func (gui *Gui) renderInstanceEnv(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.instanceEnvStr(instance) })
}

func (gui *Gui) instanceEnvStr(instance *commands.Instance) string {
	if env := presentation.InstanceEnv(instance); env != "" {
		return env
	}

	return gui.Tr.NothingToDisplay
}
