package tui

import (
	"time"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

// serviceMainTabs are pkg/gui's, in its order.
func (m *Model) serviceMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "info", title: m.tr.InfoTitle, task: m.serviceInfoTask, wrap: wrap},
		{key: "logs", title: m.tr.LogsTitle, task: m.serviceLogsTask, wrap: wrap, autoscroll: true},
		{key: "config", title: m.tr.ConfigTitle, task: m.serviceConfigTask, wrap: wrap},
		{key: "env", title: m.tr.EnvTitle, task: m.serviceInstanceTab(m.instanceEnvTask), wrap: wrap},
		{key: "top", title: m.tr.TopTitle, task: m.serviceInstanceTab(m.instanceTopTask)},
	}
}

// serviceInfoTask ticks like an instance's Info tab: the identity half is
// static, the counters under it aren't.
func (m *Model) serviceInfoTask() mainTaskFunc {
	row, holder, tr, width := m.selectedServiceRow(), m.composeState, m.tr, m.main.width

	service := row.Service
	remote := service.Remote
	if remote == "" {
		remote = m.incus.RemoteName()
	}

	loc := presentation.Location{Remote: remote, Project: service.Project, Stack: m.stackLabel(remote, service.Project)}
	verdict := presentation.UpstreamVerdict(commands.MostTelling(m.upstreams, presentation.BaseImages(row.Instances())...), tr)

	locations := map[string]presentation.Location{}
	hosts := map[string]string{}

	for _, instance := range row.Instances() {
		locations[instance.Key()] = m.instanceLocation(instance)
		hosts[instance.Key()] = m.publishHostFor(instance.Remote)
	}

	instanceInfo := func(instance *commands.Instance, omit ...string) string {
		return presentation.InstanceInfo(instance, locations[instance.Key()], hosts[instance.Key()], omit...)
	}

	return tickerTask(time.Second, func() string {
		_, project := holder.load()

		return presentation.ServiceInfo(row, loc, verdict, project, instanceInfo, width, tr)
	})
}

// serviceLogsTask delegates to the instance logs for a replica or a lone
// instance; a replicated service's own row stacks every replica's under a
// heading of its own.
func (m *Model) serviceLogsTask() mainTaskFunc {
	row, tr, width, logOf := m.selectedServiceRow(), m.tr, m.main.width, m.instanceLogText()

	if instance, ok := row.SelectedInstance(); ok {
		return m.instanceLogsTask(instance)
	}

	instances := row.Instances()
	if len(instances) == 0 {
		return renderString(func() string { return tr.ServiceNotRunning })
	}

	return tickerTask(time.Second, func() string {
		return presentation.ServiceLogs(row.Service, instances, width, logOf, tr)
	})
}

// serviceConfigTask is both halves of a service's configuration: what the
// compose file declares, then what the daemon made of it.
func (m *Model) serviceConfigTask() mainTaskFunc {
	row, incus, tr, width := m.selectedServiceRow(), m.incus, m.tr, m.main.width

	return renderString(func() string {
		definition, found, err := incus.ComposeServiceConfig(row.Service.Dir, row.Service.Name)

		return presentation.ServiceConfig(row, presentation.ComposeServiceConfig(definition, found, err, tr), width, tr)
	})
}

// serviceInstanceTab is a tab that is really the instance's: the row's
// replica answers for it, as does a lone service's only instance.
func (m *Model) serviceInstanceTab(task func(*commands.Instance) mainTaskFunc) func() mainTaskFunc {
	return func() mainTaskFunc {
		row := m.selectedServiceRow()

		instance, ok := row.SelectedInstance()
		if !ok {
			message := presentation.ServiceNoSingleInstance(row.Service, m.tr)

			return renderString(func() string { return message })
		}

		return task(instance)
	}
}

// viewServiceLogs is pkg/gui's handleServiceViewLogs.
func (m *Model) viewServiceLogs() {
	m.setMainTab(m.panelNamed("services"), "logs")
	m.enterMain()
}
