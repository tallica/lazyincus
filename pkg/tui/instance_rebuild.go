package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// instanceRebuild is `U` on Instances; a compose instance is the Services panel's U.
func (m *Model) instanceRebuild(instance *commands.Instance) tea.Cmd {
	switch {
	case instance.ComposeService() != "":
		return m.withTransientStatus(m.tr.RebuildComposeInstance, 3*time.Second)
	case !instance.IsOCI():
		return m.withTransientStatus(m.tr.RebuildNotOCI, 3*time.Second)
	case len(instance.Instance.Snapshots) > 0:
		m.openError(fmt.Sprintf(m.tr.RebuildNoSource, instance.Name, commands.ErrRebuildSnapshots))
		return nil
	}

	var source commands.OCIRebuild

	return m.withWaitingStatusThen(m.tr.CheckingImagesStatus, func() error {
		var err error
		source, err = instance.OCIRebuildSource()

		return err
	}, func(err error) tea.Cmd {
		if err != nil {
			m.openError(fmt.Sprintf(m.tr.RebuildNoSource, instance.Name, err))
			return nil
		}

		// Not over a popup opened while the image was looked up.
		if len(m.popups) > 0 {
			return nil
		}

		m.openMenu(menuOptions{title: m.tr.RebuildTitle, items: m.rebuildItems(instance, source)})

		return nil
	})
}

// rebuildItems: the tag it runs, and the newer one beacon found.
func (m *Model) rebuildItems(instance *commands.Instance, source commands.OCIRebuild) []*menuItem {
	targets := []commands.OCIRebuild{source}
	labels := []string{fmt.Sprintf(m.tr.RebuildNewestBuild, source.Image())}

	if tag := instance.BeaconNewerTag(source); tag != "" {
		targets = append(targets, source.WithTag(tag))
		labels = append(labels, fmt.Sprintf(m.tr.RebuildOntoTag, source.WithTag(tag).Image()))
	}

	items := make([]*menuItem, 0, len(targets))
	for i, target := range targets {
		items = append(items, &menuItem{label: labels[i], onPress: func(m *Model) tea.Cmd {
			return m.confirmRebuild(instance, target)
		}})
	}

	return items
}

func (m *Model) confirmRebuild(instance *commands.Instance, source commands.OCIRebuild) tea.Cmd {
	message := fmt.Sprintf(m.tr.ConfirmRebuild, m.qualifiedInstance(instance), source.Image())

	m.openConfirmation(m.tr.Confirm, message, func() tea.Cmd {
		return m.inTransition(m.tr.RebuildingStatus, instance, "Rebuilding", func() error {
			return instance.RebuildFromOCI(source)
		})
	})

	return nil
}
