package tui

import (
	"sort"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) fetchProfiles() fetch {
	ticket := m.refreshes.profiles.issue()

	return func() (func(), error) {
		profiles, err := m.incus.GetProfiles()
		if err != nil {
			return nil, err
		}

		return func() {
			if m.refreshes.profiles.admit(ticket) {
				m.setProfiles(profiles)
			}
		}, nil
	}
}

func (m *Model) setProfiles(profiles []*commands.Profile) {
	m.refill(m.panelNamed("profiles"), func(item int) string { return m.profiles[item].Key() }, func() [][]string {
		sort.SliceStable(profiles, func(a, b int) bool { return sortProfiles(profiles[a], profiles[b]) })
		m.profiles = profiles
		m.profilesSpan = spansProjects(profiles, profileProject)

		cells := make([][]string, len(profiles))
		for i, profile := range profiles {
			cells[i] = presentation.GetProfileDisplayStrings(profile, m.profilesSpan)
		}

		return cells
	})
}

func sortProfiles(a, b *commands.Profile) bool {
	if a.Profile.Project != b.Profile.Project {
		return a.Profile.Project < b.Profile.Project
	}

	return a.Name < b.Name
}

func profileProject(profile *commands.Profile) string { return profile.Profile.Project }

func (m *Model) profileFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    projectColumns(m.profilesSpan) + presentation.ProfileDescriptionColumn,
		MinWidth: presentation.MinProfileDescriptionWidth,
	}}
}

func (m *Model) selectedProfile() *commands.Profile {
	item := m.panelNamed("profiles").item()
	if item < 0 || item >= len(m.profiles) {
		return nil
	}

	return m.profiles[item]
}

func (m *Model) profileMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "devices", title: m.tr.DevicesTitle, task: m.profileDevicesTask, wrap: wrap},
		{key: "config", title: m.tr.ConfigTitle, task: m.profileConfigTask, wrap: wrap},
	}
}

func (m *Model) profileDevicesTask() mainTaskFunc {
	profile, tr := m.selectedProfile(), m.tr

	return renderString(func() string { return presentation.ProfileDevices(profile, tr) })
}

func (m *Model) profileConfigTask() mainTaskFunc {
	profile := m.selectedProfile()
	loc := m.sessionLocation(profile.Profile.Project)

	return renderString(func() string { return presentation.ProfileConfig(profile, loc) })
}

func (m *Model) profileKey() string {
	profile := m.selectedProfile()
	if profile == nil {
		return ""
	}

	return profile.Key() + "-" + utils.Fingerprint(profile.Profile)
}
