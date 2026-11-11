package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// instanceUpdate is an OCI container with a newer build or tag to take.
type instanceUpdate struct {
	instance *commands.Instance
	source   commands.OCIRebuild
	tag      string
	// status is g's newer build, State empty when only beacon saw one.
	status commands.UpstreamStatus
}

// updateInstanceImages is `G` on Instances: docs/Panels.md, "Instances".
func (m *Model) updateInstanceImages() tea.Cmd {
	var candidates []*commands.Instance

	for _, instance := range m.instances.items {
		if instance.ComposeService() == "" && instance.IsOCI() {
			candidates = append(candidates, instance)
		}
	}

	covered := beaconCoversInstances(candidates)
	checked := slices.ContainsFunc(candidates, func(instance *commands.Instance) bool {
		_, ok := m.upstreams[instance.BaseImage()]
		return ok
	})

	if checked || covered || len(candidates) == 0 {
		return m.listInstanceUpdates(candidates, !covered)
	}

	return m.waitingRefreshThen(m.tr.CheckingImagesStatus, func() tea.Cmd {
		return m.listInstanceUpdates(candidates, true)
	}, m.imageUpdatesFetch(false))
}

// listInstanceUpdates looks up where each image came from, off the loop.
func (m *Model) listInstanceUpdates(candidates []*commands.Instance, recheck bool) tea.Cmd {
	upstreams := m.upstreams

	var found, blocked []instanceUpdate

	return m.withWaitingStatusThen(m.tr.CheckingImagesStatus, func() error {
		found, blocked = instanceUpdates(candidates, upstreams)
		return nil
	}, func(error) tea.Cmd {
		// Not over a popup opened while the images were looked up.
		if len(m.popups) > 0 {
			return nil
		}

		switch {
		case len(found) > 0:
			m.openInstanceUpdates(found, blocked, recheck)
		case len(blocked) > 0:
			m.openError(fmt.Sprintf(m.tr.UpdateInstancesSnapshots, updateNames(blocked)))
		default:
			return m.withTransientStatus(m.tr.NoNewerInstanceImages, 3*time.Second)
		}

		return nil
	})
}

// instanceUpdates: blocked are those with snapshots, which Incus won't recreate.
func instanceUpdates(candidates []*commands.Instance, upstreams map[string]commands.UpstreamStatus) (found, blocked []instanceUpdate) {
	for _, instance := range candidates {
		source, _, err := instance.OCIRebuildSource()
		if err != nil {
			continue
		}

		tag := instance.BeaconNewerTag(source)
		verdict := instance.Beacon()
		rebuilt := verdict.Status == commands.BeaconUpdateAvailable && verdict.Rebuilt()

		if tag == "" && !rebuilt && commands.MostTelling(upstreams, instance.BaseImage()).State != commands.UpstreamNewer {
			continue
		}

		update := instanceUpdate{instance: instance, source: source, tag: tag}
		if status := commands.MostTelling(upstreams, instance.BaseImage()); status.State == commands.UpstreamNewer {
			update.status = status
		}
		if len(instance.Instance.Snapshots) > 0 {
			blocked = append(blocked, update)
		} else {
			found = append(found, update)
		}
	}

	return found, blocked
}

// instanceUpdateChange is the row's from → to, as G on Services words it.
func (m *Model) instanceUpdateChange(update instanceUpdate) string {
	switch {
	case update.tag != "":
		return "→ " + update.tag
	case update.status.State == commands.UpstreamNewer:
		return buildChangeStr(update.status)
	}

	return m.tr.BeaconRebuilt
}

func updateNames(updates []instanceUpdate) string {
	names := make([]string, 0, len(updates))
	for _, update := range updates {
		names = append(names, update.instance.Name)
	}

	return strings.Join(names, ", ")
}

func (m *Model) openInstanceUpdates(found, blocked []instanceUpdate, recheck bool) {
	items := make([]*menuItem, 0, len(found))
	updates := make(map[*menuItem]instanceUpdate, len(found))

	for _, update := range found {
		item := &menuItem{labelColumns: []string{update.instance.Name, update.source.Image(), m.instanceUpdateChange(update)}, checked: true}

		if update.tag == "" && update.status.State == commands.UpstreamNewer {
			if links := commands.BuildLinks(update.status.Ref, update.status.From, update.status.To); len(links) > 0 {
				item.keys = map[string]menuKey{"o": {handler: openBuildLink(links)}}
			}
		}

		items = append(items, item)
		updates[item] = update
	}

	subtitle := ""
	if len(blocked) > 0 {
		subtitle = fmt.Sprintf(m.tr.UpdateInstancesLeftOut, updateNames(blocked))
	}

	m.openMenu(menuOptions{
		title:    m.tr.ComposeUpdateImagesTitle,
		subtitle: subtitle,
		items:    items,
		onConfirm: func(picked []*menuItem) tea.Cmd {
			chosen := make([]instanceUpdate, 0, len(picked))
			for _, item := range picked {
				chosen = append(chosen, updates[item])
			}

			return m.confirmInstanceUpdates(chosen, recheck)
		},
	})
}

// confirmInstanceUpdates: one failing leaves the rest to go.
func (m *Model) confirmInstanceUpdates(chosen []instanceUpdate, recheck bool) tea.Cmd {
	names := make([]string, 0, len(chosen))
	for _, update := range chosen {
		image := update.source
		if update.tag != "" {
			image = image.WithTag(update.tag)
		}

		names = append(names, m.qualifiedInstance(update.instance)+" from "+image.Image())
	}

	message := fmt.Sprintf(m.tr.ConfirmUpdateInstances, strings.Join(names, ", "))

	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return m.withWaitingStatusThen(m.tr.RecreatingStatus, func() error {
			var failed []error

			for _, update := range chosen {
				pull := update.source
				if update.tag != "" {
					pull = pull.WithTag(update.tag)
				}

				if err := update.instance.Recreate(&pull); err != nil {
					failed = append(failed, fmt.Errorf("%s: %w", update.instance.Name, err))
				}
			}

			return errors.Join(failed...)
		}, func(err error) tea.Cmd {
			if err != nil {
				m.openError(err.Error())
			}

			if recheck {
				return tea.Batch(refresh(m.instanceFetches()...), refreshQuiet(nil, m.imageUpdatesFetch(false)))
			}

			return refresh(m.instanceFetches()...)
		})
	})

	return nil
}
