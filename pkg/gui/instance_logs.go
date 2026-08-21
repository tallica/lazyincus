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
// Incus's console log endpoint is pull-based (it returns the current contents
// of a ring buffer), so we poll it rather than streaming.
func (gui *Gui) renderInstanceLogsToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			content, err := instance.ConsoleLog()
			if err != nil {
				content = err.Error()
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
