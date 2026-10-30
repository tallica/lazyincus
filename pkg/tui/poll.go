package tui

import (
	"fmt"
	"os"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	instancesPollInterval = 2 * time.Second
	configPollInterval    = 2 * time.Second
	// resourcesPollInterval and watchedPollInterval are pkg/gui's
	// pollUnlessWatched (docs/Incus.md, "Events").
	resourcesPollInterval = 10 * time.Second
	watchedPollInterval   = time.Minute
)

type pollMsg struct{}

// slowPollMsg is a resource list's poll, by its refresh kind.
type slowPollMsg struct{ kind refreshKind }

type configPollMsg struct{}

type configStatMsg struct{ modTime time.Time }

// pollers start once the startup read is under way, as pkg/gui's do.
func (m *Model) pollers() tea.Cmd {
	return tea.Batch(
		m.tick(instancesPollInterval, pollMsg{}),
		m.tick(configPollInterval, configPollMsg{}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshImages}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshVolumes}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshNetworks}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshProfiles}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshStacks}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshBackups}),
		m.tick(resourcesPollInterval, slowPollMsg{refreshOperations}),
		m.tick(warningsPollInterval, warningsPollMsg{}),
	)
}

// slowPoll is pkg/gui's pollUnlessWatched: while the event stream is open,
// once a minute rather than every tick.
func (m *Model) slowPoll(kind refreshKind) tea.Cmd {
	next := m.tick(resourcesPollInterval, slowPollMsg{kind})
	// The session's stream says nothing of another remote's stacks.
	if m.eventsLive && (kind != refreshStacks || !m.stacksElsewhere) && time.Since(m.polledAt[kind]) < watchedPollInterval {
		return next
	}

	if m.polledAt == nil {
		m.polledAt = map[refreshKind]time.Time{}
	}
	m.polledAt[kind] = time.Now()

	return refreshQuiet(next, m.fetchesFor(kind)...)
}

// pollInstances is pkg/gui's refreshInstancesQuiet, the connection check too.
func (m *Model) pollInstances() tea.Cmd {
	return refreshQuiet(tea.Batch(syncConnection, m.tick(instancesPollInterval, pollMsg{})), m.fetchInstances(), m.fetchServices())
}

type syncConnectionMsg struct{}

func syncConnection() tea.Msg { return syncConnectionMsg{} }

// syncConnection is pkg/gui's; never raised over a popup already up.
func (m *Model) syncConnection() {
	if m.incus.IsConnected() {
		m.connectionLost = false
		m.popups = slices.DeleteFunc(m.popups, func(p *popup) bool { return p.title == m.tr.ConnectionLostTitle })

		return
	}

	if m.connectionLost || len(m.popups) > 0 {
		return
	}

	m.connectionLost = true
	m.openMessage(m.tr.ConnectionLostTitle, fmt.Sprintf(m.tr.ConnectionLost, m.incus.RemoteName()))
}

// statConfig is pkg/gui's configReloader, the file's time read off the loop.
func (m *Model) statConfig() tea.Cmd {
	filename := m.cfg.ConfigFilename()

	return func() tea.Msg {
		info, err := os.Stat(filename)
		if err != nil {
			return configStatMsg{}
		}

		return configStatMsg{modTime: info.ModTime()}
	}
}

func (m *Model) onConfigStat(modTime time.Time) tea.Cmd {
	next := m.tick(configPollInterval, configPollMsg{})
	if modTime.IsZero() {
		return next
	}

	changed := !m.configModTime.IsZero() && !modTime.Equal(m.configModTime)
	m.configModTime = modTime
	if changed {
		if err := m.reloadConfig(); err != nil {
			// Likely a half-written save; the next write reloads again.
			m.incus.Log.Warn().Err(err).Send()
		}
	}

	return next
}

// reloadConfig is pkg/gui's: only what the model caches is re-applied.
func (m *Model) reloadConfig() error {
	if err := m.cfg.ReloadUserConfig(); err != nil {
		return err
	}

	m.applyTheme()
	m.rerenderInstances()

	return nil
}
