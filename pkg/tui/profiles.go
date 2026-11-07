package tui

import (
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (m *Model) newProfiles(p *panel) list[*commands.Profile] {
	return list[*commands.Profile]{
		panel: p, seq: &m.refreshes.profiles,
		get: func(incus *commands.IncusCommand, _ *commands.Listing) ([]*commands.Profile, error) {
			return incus.GetProfiles()
		},
		less:    sortProfiles,
		key:     (*commands.Profile).Key,
		project: func(profile *commands.Profile) string { return profile.Profile.Project },
		cells:   presentation.GetProfileDisplayStrings,
		version: func(profile *commands.Profile) string { return utils.Fingerprint(profile.Profile) },
	}
}

func sortProfiles(a, b *commands.Profile) bool {
	if a.Profile.Project != b.Profile.Project {
		return a.Profile.Project < b.Profile.Project
	}

	return a.Name < b.Name
}

func (m *Model) profileFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    projectColumns(m.profiles.span) + presentation.ProfileDescriptionColumn,
		MinWidth: presentation.MinProfileDescriptionWidth,
	}}
}

func (m *Model) profileMainTabs() []mainTab {
	wrap := m.cfg.UserConfig.Gui.WrapMainPanel

	return []mainTab{
		{key: "devices", title: m.tr.DevicesTitle, task: m.profileDevicesTask, wrap: wrap},
		{key: "config", title: m.tr.ConfigTitle, task: m.profileConfigTask, wrap: wrap},
	}
}

func (m *Model) profileDevicesTask() mainTaskFunc {
	profile, tr := m.profiles.selected(), m.tr

	return renderString(func() string { return presentation.ProfileDevices(profile, tr) })
}

func (m *Model) profileConfigTask() mainTaskFunc {
	profile := m.profiles.selected()
	loc := m.sessionLocation(profile.Profile.Project)

	return renderString(func() string { return presentation.ProfileConfig(profile, loc) })
}
