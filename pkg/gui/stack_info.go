package gui

import (
	"context"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
)

type stackInstances = presentation.StackInstances

// renderStackInfo ticks for what arrives with the services once the stack
// is selected, and for the counters.
func (gui *Gui) renderStackInfo(stack *commands.ComposeStack) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.stackInfoStr(stack))
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) stackInfoStr(stack *commands.ComposeStack) string {
	return presentation.StackInfo(stack, gui.stackRemote(stack), gui.home, gui.composeInstances.Load(),
		gui.composeProject.Load(), gui.publishHostFor(stack.Remote), int(gui.mainViewWidth.Load()),
		&gui.Config.UserConfig.Gui, gui.Tr)
}
