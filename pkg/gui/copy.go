package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/types"
	"github.com/tallica/lazyincus/pkg/utils"
)

// copyValue is one thing `y` offers to copy: what it is, and the value.
type copyValue struct {
	label string
	value string
}

// copyMenu offers an item's values to copy, each shown beside its label so
// the menu says what lands on the clipboard. Empty values are left out: a
// stopped instance has no address to offer.
func (gui *Gui) copyMenu(values ...copyValue) error {
	values = lo.Filter(values, func(value copyValue, _ int) bool { return value.value != "" })
	if len(values) == 0 {
		return gui.createErrorPanel(gui.Tr.NothingToCopy)
	}

	items := lo.Map(values, func(value copyValue, _ int) *types.MenuItem {
		return &types.MenuItem{
			LabelColumns: []string{value.label, utils.ColoredString(value.value, color.FgCyan)},
			OnPress:      func() error { return gui.copyToClipboard(value.value) },
		}
	})

	return gui.Menu(CreateMenuOptions{Title: gui.Tr.CopyMenuTitle, Items: items})
}

func (gui *Gui) copyToClipboard(value string) error {
	if err := gui.OSCommand.CopyToClipboard(value); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	gui.WithTransientStatus(fmt.Sprintf("%s %s", gui.Tr.CopiedToClipboard, value), time.Second*2)

	return nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func (gui *Gui) instanceCopy(instance *commands.Instance) error {
	ipv4, ipv6 := instance.Addresses("inet"), instance.Addresses("inet6")

	all := ""
	if len(ipv4)+len(ipv6) > 1 {
		all = strings.Join(append(ipv4, ipv6...), " ")
	}

	return gui.copyMenu(
		copyValue{gui.Tr.CopyName, instance.Name},
		copyValue{"IPv4", first(ipv4)},
		copyValue{"IPv6", first(ipv6)},
		copyValue{gui.Tr.CopyAllAddresses, all},
	)
}

func (gui *Gui) handleServiceCopy(g *gocui.Gui, v *gocui.View) error {
	return gui.withServiceInstance(gui.Tr.CopyMenuTitle, gui.instanceCopy)
}

// stackCopy offers each published port at its address, named for the
// instance and the port inside it; a port whose host isn't known has none.
func (gui *Gui) stackCopy(stack *commands.ComposeStack) error {
	values := []copyValue{
		{gui.Tr.CopyProject, stack.Name},
		{gui.Tr.CopyDirectory, stack.Dir},
	}

	if state := gui.composeInstances.Load(); state != nil && state.project == stack.Name {
		for _, service := range sortedServices(state.services) {
			for _, instance := range service.SortedInstances() {
				for _, port := range instance.Latest().PublishedPorts(gui.IncusCommand.PublishHost) {
					label := endpointName(service, instance) + " → " + port.Target
					if port.Protocol != "tcp" {
						label += "/" + port.Protocol
					}

					values = append(values, copyValue{label, port.Address()})
				}
			}
		}
	}

	return gui.copyMenu(values...)
}

func (gui *Gui) imageCopy(image *commands.Image) error {
	return gui.copyMenu(
		copyValue{gui.Tr.CopyFingerprint, image.Fingerprint},
		copyValue{gui.Tr.CopyAlias, image.Alias()},
	)
}

func (gui *Gui) volumeCopy(volume *commands.Volume) error {
	return gui.copyMenu(
		copyValue{gui.Tr.CopyName, volume.Name},
		copyValue{gui.Tr.CopyPool, volume.Pool},
	)
}

func (gui *Gui) networkCopy(network *commands.Network) error {
	return gui.copyMenu(
		copyValue{gui.Tr.CopyName, network.Name},
		// The gateway with its prefix, as Incus's ipv4.address has it; not
		// the subnet's own address.
		copyValue{"IPv4 address", network.Network.Config["ipv4.address"]},
		copyValue{"IPv6 address", network.Network.Config["ipv6.address"]},
	)
}

func (gui *Gui) profileCopy(profile *commands.Profile) error {
	return gui.copyMenu(copyValue{gui.Tr.CopyName, profile.Name})
}

// snapshotCopy offers owner/snapshot too, the form `incus copy` and
// `incus storage volume copy` take a snapshot in.
func (gui *Gui) snapshotCopy(snapshot *commands.Snapshot) error {
	return gui.copyMenu(
		copyValue{gui.Tr.CopyName, snapshot.Name},
		copyValue{gui.Tr.CopySnapshotRef, snapshot.Owner + "/" + snapshot.Name},
	)
}
