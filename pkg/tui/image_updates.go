package tui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/commands"
)

// imageUpdateTimeout bounds one check of every image against the registries.
const imageUpdateTimeout = time.Minute

// imageUpdatesFetch checks the OCI images instances were created from against their registries; report says what it found.
func (m *Model) imageUpdatesFetch(report bool) fetch {
	ticket := m.refreshes.imageUpdates.issue()
	incus, check, remotes := m.incus, m.checkUpstreams, slices.Clone(m.stackRemotes)

	return func() (func(), error) {
		targets, err := m.upstreamTargets(incus, remotes)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), imageUpdateTimeout)
		defer cancel()

		results := check(ctx, targets)

		return func() {
			if !m.refreshes.imageUpdates.admit(ticket) {
				return
			}

			updates := maps.Clone(m.upstreams)
			if updates == nil {
				updates = map[string]commands.UpstreamStatus{}
			}
			maps.Copy(updates, results)
			m.upstreams = updates

			if report {
				m.queue(m.withTransientStatus(m.imageUpdatesSummary(results), 3*time.Second))
			}

			if m.images != nil {
				m.setImages(m.images)
			}
			m.rerenderStacks()
			m.rerenderServices()
			m.rerenderInstances()
		}, nil
	}
}

// upstreamTargets are the session's remote's images to check, and those of
// every other remote a listed stack is on that's connected; one that isn't
// yet is left for the next check.
func (m *Model) upstreamTargets(incus *commands.IncusCommand, remotes []string) ([]commands.UpstreamTarget, error) {
	targets, err := incus.UpstreamTargets()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, target := range targets {
		seen[target.Fingerprint] = true
	}

	for _, remote := range remotes {
		if remote == incus.RemoteName() {
			continue
		}

		command, err := m.remoteCommand(remote)
		if err == nil {
			var others []commands.UpstreamTarget
			if others, err = command.UpstreamTargets(); err == nil {
				for _, target := range others {
					if !seen[target.Fingerprint] {
						seen[target.Fingerprint] = true
						targets = append(targets, target)
					}
				}
			}
		}

		if err != nil {
			incus.Log.Warn().Err(err).Str("remote", remote).Msg("no image check")
		}
	}

	return targets, nil
}

// remoteCommand is commandFor for a remote other than the session's, which a fetch can ask off the loop.
func (m *Model) remoteCommand(remote string) (*commands.IncusCommand, error) {
	if !m.remotes.known(remote) {
		return nil, m.unknownRemote(remote)
	}

	return m.remotes.get(remote)
}

func (m *Model) imageUpdatesSummary(results map[string]commands.UpstreamStatus) string {
	if len(results) == 0 {
		return m.tr.NoImagesToCheck
	}

	counts := map[commands.UpstreamState]int{}
	for _, status := range results {
		counts[status.State]++
	}

	return fmt.Sprintf(m.tr.ImageUpdatesSummary,
		counts[commands.UpstreamNewer], counts[commands.UpstreamCurrent], counts[commands.UpstreamFailed])
}

// checkImageUpdates is pkg/gui's handleCheckImageUpdates.
func (m *Model) checkImageUpdates() tea.Cmd {
	return m.waitingRefresh(m.tr.CheckingImagesStatus, m.imageUpdatesFetch(true))
}
