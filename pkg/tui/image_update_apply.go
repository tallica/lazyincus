package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
)

// serviceUpdate is a service with a newer build, or beacon's newer tag.
type serviceUpdate struct {
	service string
	status  commands.UpstreamStatus
	bump    *commands.ComposeTagBump
	// rebuilt is beacon's newer build, g having found none.
	rebuilt string
}

func servicesWithUpdates(services []*commands.ComposeService, updates map[string]commands.UpstreamStatus) []serviceUpdate {
	var found []serviceUpdate

	for _, service := range services {
		if bump, ok := service.TagBump(); ok {
			found = append(found, serviceUpdate{service: service.Name, bump: &bump})
			continue
		}

		status := commands.MostTelling(updates, presentation.BaseImages(service.SortedInstances())...)
		if status.State == commands.UpstreamNewer {
			found = append(found, serviceUpdate{service: service.Name, status: status})
		} else if verdict := service.Beacon(); verdict.Status == commands.BeaconUpdateAvailable && verdict.Rebuilt() {
			found = append(found, serviceUpdate{service: service.Name, rebuilt: verdict.Current})
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

	if checked || beaconCovers(m.stackServiceList(stack)) {
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

// beaconCovers is incus-beacon having checked every instance of the services.
func beaconCovers(services []*commands.ComposeService) bool {
	var instances []*commands.Instance
	for _, service := range services {
		instances = append(instances, service.Instances...)
	}

	return beaconCoversInstances(instances)
}

func beaconCoversInstances(instances []*commands.Instance) bool {
	for _, instance := range instances {
		switch instance.Beacon().Status {
		case commands.BeaconCurrent, commands.BeaconUpdateAvailable:
		default:
			return false
		}
	}

	return len(instances) > 0
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
		switch {
		case update.bump != nil:
			items = append(items, &menuItem{
				labelColumns: []string{update.service, update.bump.From, "→ " + update.bump.Tag()}, checked: true,
			})

			continue
		case update.rebuilt != "":
			items = append(items, &menuItem{labelColumns: []string{update.service, update.rebuilt, m.tr.BeaconRebuilt}, checked: true})

			continue
		}

		item := &menuItem{
			labelColumns: []string{update.service, update.status.Ref, buildChangeStr(update.status)},
			checked:      true,
		}

		if links := commands.BuildLinks(update.status.Ref, update.status.From, update.status.To); len(links) > 0 {
			item.keys = map[string]menuKey{"o": {handler: openBuildLink(links)}}
		}

		items = append(items, item)
	}

	m.openMenu(menuOptions{
		title: m.tr.ComposeUpdateImagesTitle,
		items: items,
		onConfirm: func(picked []*menuItem) tea.Cmd {
			names := make([]string, 0, len(picked))
			var bumps []commands.ComposeTagBump

			for _, item := range picked {
				names = append(names, item.labelColumns[0])

				for _, update := range found {
					if update.service == item.labelColumns[0] && update.bump != nil {
						bumps = append(bumps, *update.bump)
					}
				}
			}

			recheck := !beaconCovers(m.stackServiceList(stack))
			if len(bumps) > 0 {
				return m.confirmTagBumps(stackTarget(stack), bumps, names, recheck)
			}

			return m.composeUpdateImages(stackTarget(stack), names, recheck)
		},
	})

	return nil
}

// openBuildLink opens the first of links that exists, asking off the loop.
func openBuildLink(links []string) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		command, open := m.cfg.UserConfig.OS.OpenLinkCommand, m.os.OpenLink

		return func() tea.Msg {
			return openedMsg{err: open(command, commands.FirstLink(context.Background(), links))}
		}
	}
}

func (m *Model) composeUpdateImages(target composeTarget, services []string, recheck bool) tea.Cmd {
	message := fmt.Sprintf(m.tr.ConfirmComposeUpdateImages, m.onRemote(strings.Join(services, ", "), target.remote))

	return m.openSkippableConfirmation("update-images", m.tr.Confirm, message, func() tea.Cmd {
		return m.recreateOnPull(target, services, recheck)
	})
}

// recreateOnPull: without --no-deps, `up --recreate` would recreate what the services depend on too.
func (m *Model) recreateOnPull(target composeTarget, services []string, recheck bool) tea.Cmd {
	args := append([]string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach"}, services...)

	var after func() tea.Cmd
	if recheck {
		after = func() tea.Cmd { return refreshQuiet(nil, m.imageUpdatesFetch(false)) }
	}

	return m.composeRunThen(target, args, after)
}
