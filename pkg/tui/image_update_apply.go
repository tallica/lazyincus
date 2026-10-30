package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

// serviceUpdate is a service with a newer build of an image it runs.
type serviceUpdate struct {
	service string
	status  commands.UpstreamStatus
}

// servicesWithUpdates are the services running an image with a newer build.
func servicesWithUpdates(services []*commands.ComposeService, updates map[string]commands.UpstreamStatus) []serviceUpdate {
	var found []serviceUpdate

	for _, service := range services {
		status := commands.MostTelling(updates, presentation.BaseImages(service.SortedInstances())...)
		if status.State == commands.UpstreamNewer {
			found = append(found, serviceUpdate{service: service.Name, status: status})
		}
	}

	return found
}

// buildChangeStr is a newer build's from → to, ? for a side unknown.
func buildChangeStr(status commands.UpstreamStatus) string {
	variable, from, to := commands.BuildChange(status.Ref, status.From, status.To)
	if from == "" && to == "" {
		return ""
	}

	change := cmp.Or(from, "?") + " → " + cmp.Or(to, "?")
	if variable != "" {
		return variable + " " + change
	}

	return change
}

// updateImages is `G`, checking first when none of the stack's images
// has been: its remote may have still been connecting at the last check.
func updateImages(m *Model, stack *commands.ComposeStack) tea.Cmd {
	checked := slices.ContainsFunc(m.stackServiceList(stack), func(service *commands.ComposeService) bool {
		return slices.ContainsFunc(presentation.BaseImages(service.SortedInstances()), func(fingerprint string) bool {
			_, ok := m.upstreams[fingerprint]
			return ok
		})
	})

	if checked {
		return m.updateImagesMenu(stack)
	}

	return m.waitingRefreshThen(m.tr.CheckingImagesStatus, func() tea.Cmd {
		// Not over a popup opened while the registries were asked.
		if len(m.popups) > 0 {
			return nil
		}

		return m.updateImagesMenu(stack)
	}, m.imageUpdatesFetch(false))
}

// stackServiceList are the stack's services the services panel has, by name
// as it sorts them: it may still be listing the stack selected before.
func (m *Model) stackServiceList(stack *commands.ComposeStack) []*commands.ComposeService {
	services := slices.DeleteFunc(m.composeServices(), func(service *commands.ComposeService) bool {
		return service.Dir != stack.Dir || service.Remote != stack.Remote
	})
	slices.SortFunc(services, func(a, b *commands.ComposeService) int { return cmp.Compare(a.Name, b.Name) })

	return services
}

func (m *Model) updateImagesMenu(stack *commands.ComposeStack) tea.Cmd {
	found := servicesWithUpdates(m.stackServiceList(stack), m.upstreams)
	if len(found) == 0 {
		return m.withTransientStatus(fmt.Sprintf(m.tr.NoNewerImages, stack.Name), 3*time.Second)
	}

	items := make([]*menuItem, 0, len(found))
	for _, update := range found {
		item := &menuItem{
			labelColumns: []string{update.service, update.status.Ref, buildChangeStr(update.status)},
			checked:      true,
		}

		if link := commands.BuildLink(update.status.From, update.status.To); link != "" {
			item.keys = map[string]menuKey{"o": {handler: func(m *Model) tea.Cmd {
				if err := m.os.OpenLink(link); err != nil {
					m.handleError(err)
				}

				return nil
			}}}
		}

		items = append(items, item)
	}

	m.openMenu(menuOptions{
		title: m.tr.ComposeUpdateImagesTitle,
		items: items,
		onConfirm: func(picked []*menuItem) tea.Cmd {
			names := make([]string, 0, len(picked))
			for _, item := range picked {
				names = append(names, item.labelColumns[0])
			}

			return m.composeUpdateImages(stackTarget(stack), names)
		},
	})

	return nil
}

// composeUpdateImages recreates the services on a pull; without --no-deps,
// `up --recreate` would recreate what they depend on too.
func (m *Model) composeUpdateImages(target composeTarget, services []string) tea.Cmd {
	message := fmt.Sprintf(m.tr.ConfirmComposeUpdateImages, m.onRemote(strings.Join(services, ", "), target.remote))
	args := append([]string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach"}, services...)

	return m.openSkippableConfirmation("update-images", m.tr.Confirm, message, func() tea.Cmd {
		return m.composeRunThen(target, args, func() tea.Cmd { return refreshQuiet(nil, m.imageUpdatesFetch(false)) })
	})
}
