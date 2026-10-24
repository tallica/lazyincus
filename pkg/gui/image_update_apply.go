package gui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/gui/types"
)

// serviceUpdate is a service with a newer build of an image it runs.
type serviceUpdate struct {
	service string
	status  commands.UpstreamStatus
}

// servicesWithUpdates are the services whose instances run an image with a
// newer build, in the order given.
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

// buildChangeStr is a newer build's from → to, a side the registry didn't
// say left as ?, and empty when it said nothing of either.
func buildChangeStr(status commands.UpstreamStatus) string {
	from, to := commands.BuildChange(status.From, status.To)
	if from == "" && to == "" {
		return ""
	}

	return cmp.Or(from, "?") + " → " + cmp.Or(to, "?")
}

// handleUpdateImages is `G` on Stacks and Services: the stack's services
// with a newer image build, to tick and recreate on it - checking the
// registries first if nothing has yet.
func (gui *Gui) handleUpdateImages(g *gocui.Gui, v *gocui.View) error {
	stack := gui.selectedStack.Load()
	if stack == nil || stack.Name == "" {
		return nil
	}

	return gui.withStack(stack, func(stack *commands.ComposeStack) error {
		if gui.upstreams() != nil {
			return gui.updateImagesMenu(stack)
		}

		return gui.WithWaitingStatus(gui.Tr.CheckingImagesStatus, func() error {
			return gui.refresh(func() error { return gui.updateImagesMenu(stack) }, gui.imageUpdatesFetch(false))
		})
	})
}

func (gui *Gui) updateImagesMenu(stack *commands.ComposeStack) error {
	var services []*commands.ComposeService

	for _, service := range gui.composeServices() {
		if service.Dir == stack.Dir && service.Remote == stack.Remote {
			services = append(services, service)
		}
	}

	// By name, as the services panel sorts them.
	slices.SortFunc(services, func(a, b *commands.ComposeService) int { return cmp.Compare(a.Name, b.Name) })

	found := servicesWithUpdates(services, gui.upstreams())
	if len(found) == 0 {
		gui.WithTransientStatus(fmt.Sprintf(gui.Tr.NoNewerImages, stack.Name), 3*time.Second)

		return nil
	}

	items := make([]*types.MenuItem, 0, len(found))
	for _, update := range found {
		item := &types.MenuItem{
			LabelColumns: []string{update.service, update.status.Ref, buildChangeStr(update.status)},
			Checked:      true,
		}

		if link := commands.BuildLink(update.status.From, update.status.To); link != "" {
			item.Keys = map[rune]types.MenuKey{'o': {Handler: func() error { return gui.OSCommand.OpenLink(link) }}}
		}

		items = append(items, item)
	}

	return gui.Menu(CreateMenuOptions{
		Title: gui.Tr.ComposeUpdateImagesTitle,
		Items: items,
		OnConfirm: func(picked []*types.MenuItem) error {
			names := make([]string, 0, len(picked))
			for _, item := range picked {
				names = append(names, item.LabelColumns[0])
			}

			return gui.composeUpdateImages(stackTarget(stack), names)
		},
	})
}

// composeUpdateImages recreates the services named on a fresh pull of
// their images, --no-deps keeping it to them: up would otherwise recreate
// what they depend on too. The registries are asked again after, for the
// images the services now run.
func (gui *Gui) composeUpdateImages(target composeTarget, services []string) error {
	message := fmt.Sprintf(gui.Tr.ConfirmComposeUpdateImages,
		gui.onRemote(strings.Join(services, ", "), target.remote))
	args := append([]string{"up", "--recreate", "--pull", "always", "--no-deps", "--detach"}, services...)

	return gui.createSkippableConfirmationPanel("update-images", gui.Tr.Confirm, message, func(g *gocui.Gui, v *gocui.View) error {
		if err := gui.composeRun(target, args...); err != nil {
			return err
		}

		gui.refreshInBackground(gui.imageUpdatesFetch(false))

		return nil
	})
}
