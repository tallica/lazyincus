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

		targets, err := gui.upstreamTargets()
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

			updates := maps.Clone(gui.upstreams())
			if updates == nil {
				updates = map[string]commands.UpstreamStatus{}
			}

			maps.Copy(updates, results)
			gui.imageUpdates.Store(&updates)

			if report {
				gui.WithTransientStatus(gui.imageUpdatesSummary(results), 3*time.Second)
			}

			if err := gui.Panels.Images.RerenderList(); err != nil {
				return err
			}

			if !gui.composeUnavailable() {
				if err := gui.Panels.Stacks.RerenderList(); err != nil {
					return err
				}
			}

			return gui.rerenderInstanceLists()
		}, nil
	}
}

// upstreams are the registry checks so far, nil before the first.
func (gui *Gui) upstreams() map[string]commands.UpstreamStatus {
	if updates := gui.imageUpdates.Load(); updates != nil {
		return *updates
	}

	return nil
}

// upstreamTargets are the session's remote's images to check, and those of
// every other remote a listed stack is on that's connected; one that isn't
// yet is left for the next check.
func (gui *Gui) upstreamTargets() ([]commands.UpstreamTarget, error) {
	targets, err := gui.IncusCommand.UpstreamTargets()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, target := range targets {
		seen[target.Fingerprint] = true
	}

	remotes := gui.stackRemotes.Load()
	if remotes == nil {
		return targets, nil
	}

	for _, remote := range *remotes {
		if remote == gui.IncusCommand.RemoteName() {
			continue
		}

		command, err := gui.commandFor(remote)
		if err != nil {
			gui.Log.Warn().Err(err).Str("remote", remote).Msg("no image check")
			continue
		}

		others, err := command.UpstreamTargets()
		if err != nil {
			gui.Log.Warn().Err(err).Str("remote", remote).Msg("no image check")
			continue
		}

		for _, target := range others {
			if !seen[target.Fingerprint] {
				seen[target.Fingerprint] = true
				targets = append(targets, target)
			}
		}
	}

	return targets, nil
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

	return utils.WithPadding("Upstream: ", identityPadding) + status.Ref + "\n" +
		utils.WithPadding("Registry: ", identityPadding) + gui.upstreamVerdict(status) + "\n" +
		utils.WithPadding("Checked: ", identityPadding) + status.CheckedAt.Local().Format(time.DateTime) + "\n"
}

// upstreamVerdict is what a check found, empty for none.
func (gui *Gui) upstreamVerdict(status commands.UpstreamStatus) string {
	switch status.State {
	case commands.UpstreamNewer:
		return gui.Tr.UpstreamNewer
	case commands.UpstreamCurrent:
		return gui.Tr.UpstreamCurrent
	case commands.UpstreamFailed:
		return status.Err.Error()
	}

	return ""
}
