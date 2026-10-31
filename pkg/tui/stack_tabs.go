package tui

import (
	"fmt"
	"time"

	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) stackMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "info", title: m.tr.InfoTitle, task: m.stackInfoTask, wrap: wrap},
		{key: "logs", title: m.tr.LogsTitle, task: m.stackLogsTask, wrap: wrap, autoscroll: true},
		{key: "config", title: m.tr.ConfigTitle, task: m.stackConfigTask, wrap: wrap},
	}
}

// stackInfoTask ticks for what arrives with the services once the stack is
// selected, and for the counters.
func (m *Model) stackInfoTask() mainTaskFunc {
	stack, holder, tr := m.selectedStackRow(), m.composeState, m.tr
	remote, host, home, width := m.stackRemote(stack), m.publishHostFor(stack.Remote), m.home, &m.mainWidth
	guiConfig := m.cfg.UserConfig.Gui

	return tickerTask(time.Second, func() string {
		state, project := holder.load()

		return presentation.StackInfo(stack, remote, home, state, project, host, int(width.Load()), &guiConfig, tr)
	})
}

// stackLogsTask stacks every service's logs, each instance under a heading of its own.
func (m *Model) stackLogsTask() mainTaskFunc {
	stack, holder, tr, width, logOf := m.selectedStackRow(), m.composeState, m.tr, &m.mainWidth, m.instanceLogText()

	return tickerTask(time.Second, func() string {
		state, _ := holder.load()

		return presentation.StackLogs(stack, state, int(width.Load()), logOf, tr)
	})
}

func (m *Model) stackConfigTask() mainTaskFunc {
	dir, incus := m.selectedStackRow().Dir, m.incus

	return renderString(func() string {
		config, err := incus.ComposeStackConfig(dir)
		if err != nil {
			return fmt.Sprintf("Error running `incus-compose config`: %v", err)
		}

		return utils.ColoredYamlString(config)
	})
}

func (m *Model) viewStackLogs() {
	m.setMainTab(m.panelNamed("stacks"), "logs")
	m.enterMain()
}
