package tui

import (
	"errors"
	"fmt"
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
}

// updateInstanceImages is `G` on Instances: docs/Panels.md, "Instances".
func (m *Model) updateInstanceImages() tea.Cmd {
	var candidates []*commands.Instance

	for _, instance := range m.instances.items {
		if instance.ComposeService() == "" && instance.IsOCI() && len(instance.Instance.Snapshots) == 0 {
			candidates = append(candidates, instance)
		}
	}

	upstreams := m.upstreams

	var found []instanceUpdate

	return m.withWaitingStatusThen(m.tr.CheckingImagesStatus, func() error {
		found = instanceUpdates(candidates, upstreams)
		return nil
	}, func(error) tea.Cmd {
		if len(found) == 0 {
			return m.withTransientStatus(m.tr.NoNewerInstanceImages, 3*time.Second)
		}

		// Not over a popup opened while the images were looked up.
		if len(m.popups) > 0 {
			return nil
		}

		m.openInstanceUpdates(found)

		return nil
	})
}

// instanceUpdates looks up where each image came from, off the loop.
func instanceUpdates(candidates []*commands.Instance, upstreams map[string]commands.UpstreamStatus) []instanceUpdate {
	var found []instanceUpdate

	for _, instance := range candidates {
		source, _, err := instance.OCIRebuildSource()
		if err != nil {
			continue
		}

		tag := instance.BeaconNewerTag(source)
		verdict := instance.Beacon()
		rebuilt := verdict.Status == commands.BeaconUpdateAvailable && verdict.Rebuilt()

		if tag != "" || rebuilt || commands.MostTelling(upstreams, instance.BaseImage()).State == commands.UpstreamNewer {
			found = append(found, instanceUpdate{instance: instance, source: source, tag: tag})
		}
	}

	return found
}

func (m *Model) openInstanceUpdates(found []instanceUpdate) {
	items := make([]*menuItem, 0, len(found))
	updates := make(map[*menuItem]instanceUpdate, len(found))

	for _, update := range found {
		change := m.tr.NewerBuild
		if update.tag != "" {
			change = "→ " + update.tag
		}

		item := &menuItem{labelColumns: []string{update.instance.Name, update.source.Image(), change}, checked: true}
		items = append(items, item)
		updates[item] = update
	}

	m.openMenu(menuOptions{
		title: m.tr.ComposeUpdateImagesTitle,
		items: items,
		onConfirm: func(picked []*menuItem) tea.Cmd {
			chosen := make([]instanceUpdate, 0, len(picked))
			for _, item := range picked {
				chosen = append(chosen, updates[item])
			}

			return m.confirmInstanceUpdates(chosen)
		},
	})
}

// confirmInstanceUpdates: one failing leaves the rest to go.
func (m *Model) confirmInstanceUpdates(chosen []instanceUpdate) tea.Cmd {
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

			return refresh(m.instanceFetches()...)
		})
	})

	return nil
}
