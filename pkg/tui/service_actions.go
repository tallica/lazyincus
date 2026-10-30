package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

func serviceTarget(service *commands.ComposeService) composeTarget {
	return composeTarget{remote: service.Remote, dir: service.Dir, project: service.Project, service: service.Name}
}

// onService is pkg/gui's selectedService: the service whatever row is selected, for the verbs only a service has.
func onService(action func(*Model, *commands.ComposeService) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		row := m.selectedServiceRow()
		if row == nil {
			return nil
		}

		return action(m, row.Service)
	}
}

func onServiceTarget(action func(*Model, composeTarget) tea.Cmd) func(*Model) tea.Cmd {
	return onService(func(m *Model, service *commands.ComposeService) tea.Cmd { return action(m, serviceTarget(service)) })
}

// onServiceRow is pkg/gui's: only a replica's row takes the instance side, a lone instance's service row is still the compose scope.
func onServiceRow(
	instanceAction func(*Model, *commands.Instance) tea.Cmd,
	serviceAction func(*Model, *commands.ComposeService) tea.Cmd,
) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		row := m.selectedServiceRow()
		if row == nil {
			return nil
		}

		if row.Instance != nil {
			return instanceAction(m, row.Instance)
		}

		return serviceAction(m, row.Service)
	}
}

// composeRowDescription is what a key with two scopes does from the selected row: `d` downs a service but deletes a replica.
func (m *Model) composeRowDescription(service, replica string) string {
	if row := m.selectedServiceRow(); row != nil && row.Instance != nil {
		return replica
	}

	return service
}

// serviceScopedDescription is a verb only a service has, said so on a replica's row.
func (m *Model) serviceScopedDescription(description string) string {
	return m.composeRowDescription(description, fmt.Sprintf(m.tr.ComposeServiceScoped, description))
}

func (m *Model) instanceForceStop(instance *commands.Instance) tea.Cmd {
	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(m.tr.ForceStopInstance, m.qualifiedInstance(instance)), func() tea.Cmd {
		return m.inTransition(m.tr.StoppingStatus, instance, "Stopping", instance.ForceStop)
	})

	return nil
}

func (m *Model) composeServices() []*commands.ComposeService {
	services := make([]*commands.ComposeService, 0, len(m.serviceRows))

	for _, row := range m.serviceRows {
		if row.Instance == nil {
			services = append(services, row.Service)
		}
	}

	return services
}

// serviceStart is `S`; incus-compose leaves a service's stopped dependencies alone unless asked, so a menu asks.
func serviceStart(m *Model, service *commands.ComposeService) tea.Cmd {
	target := serviceTarget(service)

	stopped := service.StoppedDependencies(m.composeServices())
	if len(stopped) == 0 {
		return m.composeRun(target, "start")
	}

	m.openMenu(menuOptions{
		title: m.tr.ComposeStartMenuTitle,
		items: []*menuItem{
			{label: fmt.Sprintf(m.tr.ComposeStartWithDeps, service.Name, strings.Join(stopped, ", ")), onPress: func(m *Model) tea.Cmd {
				return m.composeRun(target, "start", "--with-deps")
			}},
			{label: fmt.Sprintf(m.tr.ComposeStartOnly, service.Name), onPress: func(m *Model) tea.Cmd {
				return m.composeRun(target, "start")
			}},
		},
	})

	return nil
}

func servicePause(m *Model, service *commands.ComposeService) tea.Cmd {
	if len(service.Instances) == 0 {
		m.openError(m.tr.ServiceNotRunning)
		return nil
	}

	return m.composeRun(serviceTarget(service), composePauseVerb(service.Status()))
}

// withServiceInstance runs an instance action on the row's instance, asking which first on a service's own row with replicas.
func withServiceInstance(title func(*Model) string, action func(*Model, *commands.Instance) tea.Cmd) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		row := m.selectedServiceRow()
		if row == nil {
			return nil
		}

		if len(row.Service.Instances) == 0 {
			m.openError(m.tr.ServiceNotRunning)
			return nil
		}

		if instance, ok := row.SelectedInstance(); ok {
			return action(m, instance)
		}

		var items []*menuItem
		for _, instance := range row.Service.SortedInstances() {
			items = append(items, &menuItem{label: instance.Name, onPress: func(m *Model) tea.Cmd { return action(m, instance) }})
		}

		m.openMenu(menuOptions{title: title(m), items: items})

		return nil
	}
}

// serviceBindings is pkg/gui's servicesKeybindings, in the order the options menu lists them for the selected row.
func (m *Model) serviceBindings() []*binding {
	service := func(key, description string, mutates bool, handler func(*Model) tea.Cmd) *binding {
		return &binding{view: "services", key: key, description: description, mutates: mutates, handler: handler}
	}
	instance := func(title func(*Model) string, action func(*Model, *commands.Instance) tea.Cmd) func(*Model) tea.Cmd {
		return withServiceInstance(title, action)
	}
	title := func(s string) func(*Model) string { return func(*Model) string { return s } }

	bindings := []*binding{
		service("S", m.tr.Start, true, onServiceRow((*Model).instanceStart, serviceStart)),
		service("s", m.tr.Stop, true, onServiceRow((*Model).instanceStop, func(m *Model, s *commands.ComposeService) tea.Cmd {
			return m.composeConfirm(m.tr.ConfirmComposeStop, serviceTarget(s), "stop")
		})),
		service("r", m.tr.Restart, true, onServiceRow((*Model).instanceRestart, func(m *Model, s *commands.ComposeService) tea.Cmd {
			return m.composeRun(serviceTarget(s), "restart")
		})),
		service("p", m.tr.Pause, true, onServiceRow((*Model).instancePauseResume, servicePause)),
		service("d", m.composeRowDescription(m.tr.ComposeDown, m.tr.Remove), true,
			onServiceRow((*Model).instanceDelete, func(m *Model, s *commands.ComposeService) tea.Cmd {
				return composeDownMenu(m, serviceTarget(s))
			})),
		service("f", m.composeRowDescription(m.tr.ComposeKill, m.tr.ForceStop), true,
			onServiceRow((*Model).instanceForceStop, func(m *Model, s *commands.ComposeService) tea.Cmd {
				return m.composeConfirm(m.tr.ConfirmComposeKill, serviceTarget(s), "kill")
			})),
		service("n", m.tr.NewSnapshot, true, instance(title(m.tr.NewSnapshot), (*Model).snapshotCreatePrompt)),
		service("m", m.tr.ViewLogs, false, do((*Model).viewServiceLogs)),
		service("y", m.tr.Copy, false, instance(title(m.tr.CopyMenuTitle), (*Model).instanceCopy)),
		service("c", m.tr.EditInEditor, true, instance(title(m.tr.EditInEditor), (*Model).instanceEdit)),
		service("F", m.tr.EditFile, true, instance(title(m.tr.EditFile), (*Model).instanceEditFile)),
		service("", m.tr.EditDescription, true, instance(title(m.tr.EditDescription), (*Model).instanceEditDescription)),
		service("E", m.tr.ExecShell, true, instance(title(m.tr.ExecShell), (*Model).instanceExecShell)),
		service("u", m.serviceScopedDescription(m.tr.ComposeUp), true, onServiceTarget(composeUp)),
		service("U", m.serviceScopedDescription(m.tr.ComposeRecreate), true, onServiceTarget(composeRecreateMenu)),
		service("b", m.serviceScopedDescription(m.tr.ComposeBuild), true, onServiceTarget(composeVerb("", "build"))),
		service("g", m.serviceScopedDescription(m.tr.ComposePull), true, onServiceTarget(composeVerb("", "pull"))),
		// The stack's, whichever row is selected.
		{view: "services", key: "G", description: m.tr.ComposeUpdateImages, mutates: true, noSelection: true, handler: onStack(updateImages)},
		service("M", m.serviceScopedDescription(m.tr.ComposeLogs), false, onServiceTarget(composeVerb("", "logs", "--follow"))),
	}

	order := "udUSsrpfbgmMnEy"
	if row := m.selectedServiceRow(); row != nil && row.Instance != nil {
		order = "SsrpdfnmMyEuUbg"
	}

	return orderByKey(bindings, order)
}

// orderByKey lists bindings in the given key order; any the order doesn't name keeps its declared place at the end.
func orderByKey(bindings []*binding, order string) []*binding {
	rank := func(b *binding) int {
		if hasRune(b.key) {
			if index := strings.Index(order, b.key); index >= 0 {
				return index
			}
		}

		return len(order)
	}

	sort.SliceStable(bindings, func(i, j int) bool { return rank(bindings[i]) < rank(bindings[j]) })

	return bindings
}
