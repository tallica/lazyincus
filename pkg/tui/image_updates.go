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
	src, check, remotes := m.sources(), m.checkUpstreams, slices.Clone(m.stackRemotes)

	return func() (func(), error) {
		targets, err := src.upstreamTargets(remotes)
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

			if m.images.items != nil {
				m.images.set(m, m.images.items)
			}
			m.rerenderStacks()
			m.serviceRows.set(m, m.serviceRows.items)
			m.rerenderInstances()
		}, nil
	}
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

func (m *Model) checkImageUpdates() tea.Cmd {
	return m.waitingRefresh(m.tr.CheckingImagesStatus, m.imageUpdatesFetch(true))
}
