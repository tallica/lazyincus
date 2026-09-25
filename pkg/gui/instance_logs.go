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

func (gui *Gui) renderInstanceLogsToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.renderLogsToMain(func() string { return gui.instanceLogStr(instance) })
}

func (gui *Gui) renderLogsToMain(content func() string) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, content())
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: true,
	})
}

// instanceLogStr reads TailConsoleLog rather than ConsoleLog: the endpoint
// drains on read, so rendering each raw snapshot would blank the panel on
// every tick with nothing new buffered.
func (gui *Gui) instanceLogStr(instance *commands.Instance) string {
	content, err := instance.TailConsoleLog()
	if err != nil {
		gui.Log.Warn(err)
	}

	if content == "" {
		return gui.Tr.NothingToDisplay
	}

	return content
}

func (gui *Gui) promptToReturn() {
	if !gui.Config.UserConfig.Gui.ReturnImmediately {
		fmt.Fprintf(os.Stdout, "\n\n%s", utils.ColoredString(gui.Tr.PressEnterToReturn, color.FgGreen))

		if _, err := fmt.Scanln(); err != nil {
			gui.Log.Error(err)
		}
	}
}
