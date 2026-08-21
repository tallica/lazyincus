package gui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// renderInstanceLogsToMain periodically re-fetches the instance's console log
// and renders it to the main panel. Unlike Docker's `container logs --follow`,
// Incus's console log endpoint is pull-based and drains newly-buffered bytes
// on each read rather than returning the full accumulated content (see
// Instance.ConsoleLog's doc comment), so we accumulate client-side via
// TailConsoleLog rather than replacing the display with each raw snapshot -
// otherwise logs flicker to "nothing to display" on every tick where
// nothing new happened to be buffered since the last poll.
func (gui *Gui) renderInstanceLogsToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			content, err := instance.TailConsoleLog()
			if err != nil {
				gui.Log.Warn(err)
			}
			if content == "" {
				content = gui.Tr.NothingToDisplay
			}

			gui.reRenderStringMain(content)
		},
		Duration:   time.Second,
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: true,
	})
}

func (gui *Gui) promptToReturn() {
	if !gui.Config.UserConfig.Gui.ReturnImmediately {
		fmt.Fprintf(os.Stdout, "\n\n%s", utils.ColoredString(gui.Tr.PressEnterToReturn, color.FgGreen))

		if _, err := fmt.Scanln(); err != nil {
			gui.Log.Error(err)
		}
	}
}
