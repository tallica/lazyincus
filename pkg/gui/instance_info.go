package gui

import (
	"context"
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
)

// renderInstanceInfoToMain periodically re-renders what the instance is and
// what it's doing: identity, then the Stats section underneath, both from
// the newest instance listing - no API calls of its own, unlike the Logs
// tab.
func (gui *Gui) renderInstanceInfoToMain(instance *commands.Instance) tasks.TaskFunc {
	return gui.NewTickerTask(TickerTaskOpts{
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gui.reRenderMain(ctx, gui.instanceInfoStr(instance.Latest()))
		},
		Duration:   time.Second,
		Before:     gui.clearMain,
		Wrap:       gui.Config.UserConfig.Gui.WrapMainPanel,
		Autoscroll: false,
	})
}

func (gui *Gui) instanceInfoStr(instance *commands.Instance, omit ...string) string {
	return presentation.InstanceInfo(instance, gui.instanceLocation(instance), gui.publishHostFor(instance.Remote), omit...)
}

func (gui *Gui) instanceIdentityStr(instance *commands.Instance, omit ...string) string {
	return presentation.InstanceIdentity(instance, gui.instanceLocation(instance), gui.publishHostFor(instance.Remote), omit...)
}

func (gui *Gui) sectionHeading(title string) string {
	return presentation.SectionHeading(title, int(gui.mainViewWidth.Load()))
}

const identityPadding = presentation.IdentityPadding
