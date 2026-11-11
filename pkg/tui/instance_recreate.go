package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// instanceRecreate is `U` on Instances; a compose instance is the Services panel's U.
func (m *Model) instanceRecreate(instance *commands.Instance) tea.Cmd {
	switch {
	case instance.ComposeService() != "":
		return m.withTransientStatus(m.tr.RecreateComposeInstance, 3*time.Second)
	case !instance.IsOCI():
		return m.withTransientStatus(m.tr.RecreateNotOCI, 3*time.Second)
	case len(instance.Instance.Snapshots) > 0:
		m.openError(fmt.Sprintf(m.tr.RecreateSnapshots, instance.Name, len(instance.Instance.Snapshots)))
		return nil
	}

	var (
		source commands.OCIRebuild
		held   bool
	)

	return m.withWaitingStatusThen(m.tr.CheckingImagesStatus, func() error {
		var err error
		source, held, err = instance.OCIRebuildSource()

		return err
	}, func(err error) tea.Cmd {
		if err != nil {
			m.openError(fmt.Sprintf(m.tr.RecreateNoSource, instance.Name, err))
			return nil
		}

		// Not over a popup opened while the image was looked up.
		if len(m.popups) > 0 {
			return nil
		}

		m.openMenu(menuOptions{title: m.tr.RecreateTitle, items: m.recreateItems(instance, source, held)})

		return nil
	})
}

// recreateItems are U on Services' two, and beacon's newer tag.
func (m *Model) recreateItems(instance *commands.Instance, source commands.OCIRebuild, held bool) []*menuItem {
	item := func(label, image string, pull *commands.OCIRebuild) *menuItem {
		return &menuItem{label: label, onPress: func(m *Model) tea.Cmd { return m.confirmRecreate(instance, image, pull) }}
	}

	var items []*menuItem
	if held {
		items = append(items, item(m.tr.RecreateCurrentImage, source.Image(), nil))
	}

	items = append(items, item(fmt.Sprintf(m.tr.RecreatePullFirst, source.Image()), source.Image(), &source))

	if tag := instance.BeaconNewerTag(source); tag != "" {
		newer := source.WithTag(tag)
		items = append(items, item(fmt.Sprintf(m.tr.RecreateOntoTag, newer.Image()), newer.Image(), &newer))
	}

	return items
}

func (m *Model) confirmRecreate(instance *commands.Instance, image string, pull *commands.OCIRebuild) tea.Cmd {
	message := m.tr.ConfirmRecreatePulled
	if pull == nil {
		message = m.tr.ConfirmRecreateCurrent
	}

	m.openConfirmation(m.tr.Confirm, fmt.Sprintf(message, m.qualifiedInstance(instance), image), func() tea.Cmd {
		return m.inTransition(m.tr.RecreatingStatus, instance, "Recreating", func() error {
			return instance.Recreate(pull)
		})
	})

	return nil
}
