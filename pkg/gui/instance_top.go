package gui

import (
	"context"
	"errors"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// renderInstanceTopToMain polls the instance's process list. Slower than the
// Stats tab's tick because each refresh execs `ps` inside the instance rather
// than reading state the background refresh already has.
func (gui *Gui) renderInstanceTopToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderStringMain(gui.instanceTopStr(instance))
		},
		Duration:   time.Second * 2,
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Wrap:       false,
		Autoscroll: false,
	})
}

func (gui *Gui) instanceTopStr(instance *commands.Instance) string {
	output, err := instance.Top()
	if errors.Is(err, commands.ErrInstanceNotRunning) {
		return gui.Tr.CannotListProcessesStoppedInstance
	}

	if err != nil {
		return gui.Tr.CannotListProcesses + "\n\n" + err.Error()
	}

	if output == "" {
		return gui.Tr.NothingToDisplay
	}

	return output
}
