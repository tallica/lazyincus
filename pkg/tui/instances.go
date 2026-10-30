package tui

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) fetchInstances() fetch {
	ticket := m.refreshes.instances.issue()

	return func() (func(), error) {
		instances, err := m.incus.GetInstances()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.instances.admit(ticket) {
				m.setInstances(instances)
			}
		}, nil
	}
}

func (m *Model) selectedInstance() *commands.Instance {
	item := m.panelNamed("instances").item()
	if item < 0 || item >= len(m.instances) {
		return nil
	}

	return m.instances[item]
}

// setInstances keeps the selection on the same instance across a refresh.
func (m *Model) setInstances(instances []*commands.Instance) {
	key := ""
	if selected := m.selectedInstance(); selected != nil {
		key = selected.Key()
	}

	sort.SliceStable(instances, func(a, b int) bool { return sortInstances(instances[a], instances[b]) })
	m.instances = instances
	m.setInstancesSpan()

	p := m.panelNamed("instances")
	p.awaiting = ""
	p.cells = m.instanceCells()
	m.filterList(p)
	for row, item := range p.shown {
		if instances[item].Key() == key {
			p.selected = row
			break
		}
	}
	p.selectRow(p.selected, m.listRows(p))

	// The snapshots come with the instances.
	m.setSnapshots()
}

// selectInstance: the cursor onto the instance with key, if shown.
func (m *Model) selectInstance(key string) bool {
	p := m.panelNamed("instances")
	for row, item := range p.shown {
		if m.instances[item].Key() == key {
			p.selectRow(row, m.listRows(p))
			return true
		}
	}

	return false
}

// rerenderInstances shows the rows' marks, which their status reads.
func (m *Model) rerenderInstances() {
	if m.instances != nil {
		m.setInstances(m.instances)
	}
}

func sortInstances(a, b *commands.Instance) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}

	if isStopped(a) != isStopped(b) {
		return isStopped(b)
	}

	return a.Name < b.Name
}

func isStopped(instance *commands.Instance) bool {
	return strings.EqualFold(instance.Status(), "Stopped")
}

// spansProjects is whether items come from more than one project.
func spansProjects[T any](items []T, project func(T) string) bool {
	seen := ""
	for _, item := range items {
		switch p := project(item); {
		case p == "":
		case seen == "":
			seen = p
		case p != seen:
			return true
		}
	}

	return false
}

func instanceProject(instance *commands.Instance) string { return instance.Project }

// keepInstance keeps every user while the list is narrowed.
func (m *Model) keepInstance(item int) bool {
	if m.instanceUsers != nil {
		return m.instanceUsers.uses(m.instances[item])
	}

	if !m.showStopped && isStopped(m.instances[item]) {
		return false
	}

	return m.showStackInstances || !m.isStackInstance(m.instances[item])
}

// isStackInstance is whether a listed stack's service claims the instance.
func (m *Model) isStackInstance(instance *commands.Instance) bool {
	return m.stackServices[instance.Project][instance.ComposeService()]
}

// setInstancesSpan is computed over the instances Instances shows.
func (m *Model) setInstancesSpan() {
	standalone := lo.Reject(m.instances, func(instance *commands.Instance, _ int) bool {
		return !m.showStackInstances && m.isStackInstance(instance)
	})

	m.showProject = spansProjects(standalone, instanceProject)
}

func (m *Model) toggleStackInstances() {
	m.showStackInstances = !m.showStackInstances
	m.rerenderInstances()
}

func (m *Model) toggleStoppedInstances() {
	m.showStopped = !m.showStopped
	m.filterList(m.panelNamed("instances"))
}

func (m *Model) instanceCells() [][]string {
	cells := make([][]string, len(m.instances))
	for i, instance := range m.instances {
		cells[i] = presentation.GetInstanceDisplayStrings(&m.cfg.UserConfig.Gui, instance, m.showProject, m.upstreams)
	}

	return cells
}

func (m *Model) instanceFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    presentation.InstanceImageColumn(&m.cfg.UserConfig.Gui, m.showProject),
		MinWidth: presentation.MinImageAliasWidth,
	}}
}

func (m *Model) instanceMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "info", title: m.tr.InfoTitle, task: m.instanceInfoTask, wrap: wrap},
		{key: "logs", title: m.tr.LogsTitle, task: m.ofSelectedInstance(m.instanceLogsTask), wrap: wrap, autoscroll: true},
		{key: "config", title: m.tr.ConfigTitle, task: m.instanceTask(presentation.InstanceConfig), wrap: wrap},
		{key: "env", title: m.tr.EnvTitle, task: m.ofSelectedInstance(m.instanceEnvTask), wrap: wrap},
		{key: "top", title: m.tr.TopTitle, task: m.ofSelectedInstance(m.instanceTopTask)},
	}
}

// instanceTask renders content for the instance selected when it starts.
func (m *Model) instanceTask(content func(*commands.Instance) string) func() mainTaskFunc {
	return func() mainTaskFunc {
		instance := m.selectedInstance()

		return renderString(func() string { return content(instance) })
	}
}

// instanceInfoTask redraws from the newest listing, asking the daemon nothing.
func (m *Model) instanceInfoTask() mainTaskFunc {
	instance := m.selectedInstance()
	loc, host := m.instanceLocation(instance), m.publishHostFor(instance.Remote)

	return tickerTask(time.Second, func() string {
		return presentation.InstanceInfo(instance.Latest(), loc, host)
	})
}

// ofSelectedInstance is a tab of the instance selected when its task starts.
func (m *Model) ofSelectedInstance(task func(*commands.Instance) mainTaskFunc) func() mainTaskFunc {
	return func() mainTaskFunc { return task(m.selectedInstance()) }
}

func (m *Model) instanceLogsTask(instance *commands.Instance) mainTaskFunc {
	logOf := m.instanceLogText()

	return tickerTask(time.Second, func() string { return logOf(instance) })
}

// instanceLogText reads TailConsoleLog rather than ConsoleLog: a running
// container's log drains on read, so a raw snapshot would blank the panel.
func (m *Model) instanceLogText() func(*commands.Instance) string {
	log, nothing := m.incus.Log, m.tr.NothingToDisplay

	return func(instance *commands.Instance) string {
		content, err := instance.TailConsoleLog()
		if err != nil {
			log.Warn().Err(err).Send()
		}

		if content == "" {
			return nothing
		}

		return utils.ConsoleText(content)
	}
}

// instanceTopTask polls slower than Info: each poll execs `ps` in the instance.
func (m *Model) instanceTopTask(instance *commands.Instance) mainTaskFunc {
	tr := m.tr

	return tickerTask(2*time.Second, func() string {
		output, err := instance.Top()
		switch {
		case errors.Is(err, commands.ErrInstanceNotRunning):
			return tr.CannotListProcessesStoppedInstance
		case err != nil:
			return tr.CannotListProcesses + "\n\n" + err.Error()
		case output == "":
			return tr.NothingToDisplay
		}

		return output
	})
}

func (m *Model) instanceEnvTask(instance *commands.Instance) mainTaskFunc {
	return renderString(func() string { return m.instanceEnv(instance) })
}

func (m *Model) instanceEnv(instance *commands.Instance) string {
	if env := presentation.InstanceEnv(instance); env != "" {
		return env
	}

	return m.tr.NothingToDisplay
}

func (m *Model) instanceLocation(instance *commands.Instance) presentation.Location {
	loc := presentation.Location{Remote: instance.Remote, Project: instance.Project}
	if loc.Remote == "" {
		loc.Remote = m.incus.RemoteName()
	}

	if service := instance.ComposeService(); service != "" {
		loc.Stack, loc.Service = m.stackLabel(loc.Remote, instance.Project), service
	}

	return loc
}

func (m *Model) instanceKey() string {
	instance := m.selectedInstance()
	if instance == nil {
		return ""
	}

	return instance.Key() + "-" + instance.Instance.Status + "-" + instance.ConfigFingerprint()
}

func (m *Model) viewInstanceLogs() {
	m.setMainTab(m.panelNamed("instances"), "logs")
	m.enterMain()
}

// instanceUsers: what the instances panel is narrowed to by `u`.
type instanceUsers struct {
	label string
	uses  func(*commands.Instance) bool
	// from is the list `u` was pressed in, which esc returns to.
	from string
}

// showUsers: Instances narrowed to what uses a resource, focused.
func (m *Model) showUsers(label string, uses func(*commands.Instance) bool) tea.Cmd {
	if !slices.ContainsFunc(m.instances, uses) {
		m.openError(fmt.Sprintf(m.tr.NothingUses, label))
		return nil
	}

	m.instanceUsers = &instanceUsers{label: label, uses: uses, from: m.currentView()}
	p := m.panelNamed("instances")
	p.selected = 0
	m.filterList(p)
	p.selectRow(0, m.listRows(p))
	m.focusView("instances")

	return nil
}

// clearInstanceUsers: the whole list back, and focus where `u` was pressed.
func (m *Model) clearInstanceUsers() {
	from := m.instanceUsers.from
	m.instanceUsers = nil
	p := m.panelNamed("instances")
	m.filterList(p)
	p.selectRow(p.selected, m.listRows(p))

	if from != "" {
		m.focusView(from)
	}
}

func (m *Model) instancesTitle() string {
	if m.instanceUsers != nil {
		return fmt.Sprintf(m.tr.InstancesUsing, m.instanceUsers.label)
	}

	if len(m.stackServices) == 0 || m.showStackInstances {
		return m.tr.InstancesTitle
	}

	return m.tr.StandaloneInstancesTitle
}
