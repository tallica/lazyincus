package gui

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// imageUpdateTimeout bounds one check of every image against the registries.
const imageUpdateTimeout = time.Minute

// imageUpdatesFetch checks the OCI images instances were created from
// against their registries; report says what it found in the status bar.
func (gui *Gui) imageUpdatesFetch(report bool) fetch {
	return func() (func() error, error) {
		ticket := gui.refreshes.imageUpdates.issue()

		targets, err := gui.IncusCommand.UpstreamTargets()
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), imageUpdateTimeout)
		defer cancel()

		results := gui.checkUpstreams(ctx, targets)

		return func() error {
			if !gui.refreshes.imageUpdates.admit(ticket) {
				return nil
			}

			if gui.State.ImageUpdates == nil {
				gui.State.ImageUpdates = map[string]commands.UpstreamStatus{}
			}

			maps.Copy(gui.State.ImageUpdates, results)

			if report {
				gui.WithTransientStatus(gui.imageUpdatesSummary(results), 3*time.Second)
			}

			if err := gui.Panels.Images.RerenderList(); err != nil {
				return err
			}

			return gui.rerenderInstanceLists()
		}, nil
	}
}

func (gui *Gui) imageUpdatesSummary(results map[string]commands.UpstreamStatus) string {
	if len(results) == 0 {
		return gui.Tr.NoImagesToCheck
	}

	counts := map[commands.UpstreamState]int{}
	for _, status := range results {
		counts[status.State]++
	}

	return fmt.Sprintf(gui.Tr.ImageUpdatesSummary,
		counts[commands.UpstreamNewer], counts[commands.UpstreamCurrent], counts[commands.UpstreamFailed])
}

func (gui *Gui) handleCheckImageUpdates(g *gocui.Gui, v *gocui.View) error {
	return gui.WithWaitingStatus(gui.Tr.CheckingImagesStatus, func() error {
		return gui.refresh(nil, gui.imageUpdatesFetch(true))
	})
}

// imageUpstreamStr is the image's registry check for its Config tab, empty
// when none has been made.
func (gui *Gui) imageUpstreamStr(status commands.UpstreamStatus) string {
	if status.State == 0 {
		return ""
	}

	var verdict string

	switch status.State {
	case commands.UpstreamNewer:
		verdict = gui.Tr.UpstreamNewer
	case commands.UpstreamCurrent:
		verdict = gui.Tr.UpstreamCurrent
	case commands.UpstreamFailed:
		verdict = status.Err.Error()
	}

	return utils.WithPadding("Upstream: ", identityPadding) + status.Ref + "\n" +
		utils.WithPadding("Registry: ", identityPadding) + verdict + "\n" +
		utils.WithPadding("Checked: ", identityPadding) + status.CheckedAt.Local().Format(time.DateTime) + "\n"
}
