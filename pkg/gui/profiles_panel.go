package gui

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/panels"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/tasks"
	"github.com/tallica/lazyincus/pkg/utils"
)

// defaultProfile is the profile every project has, and keeps.
const defaultProfile = "default"

func (gui *Gui) getProfilesPanel() *panels.SideListPanel[*commands.Profile] {
	return &panels.SideListPanel[*commands.Profile]{
		ContextState: &panels.ContextState[*commands.Profile]{
			GetMainTabs: func() []panels.MainTab[*commands.Profile] {
				return []panels.MainTab[*commands.Profile]{
					{
						Key:    "devices",
						Title:  gui.Tr.DevicesTitle,
						Render: gui.renderProfileDevices,
					},
					{
						Key:    "config",
						Title:  gui.Tr.ConfigTitle,
						Render: gui.renderProfileConfig,
					},
				}
			},
			GetItemContextCacheKey: func(profile *commands.Profile) string {
				return "profiles-" + profile.Key() + "-" + utils.Fingerprint(profile.Profile)
			},
		},
		ListPanel: panels.ListPanel[*commands.Profile]{
			List: panels.NewFilteredList[*commands.Profile](),
			View: gui.Views.Profiles,
		},
		NoItemsMessage: gui.Tr.NoProfiles,
		Gui:            gui.intoInterface(),
		Sort: func(a *commands.Profile, b *commands.Profile) bool {
			if a.Profile.Project != b.Profile.Project {
				return a.Profile.Project < b.Profile.Project
			}

			return a.Name < b.Name
		},
		SameItem: func(a, b *commands.Profile) bool {
			return a.Key() == b.Key()
		},
		GetTableCells: func(item *commands.Profile) []string {
			return presentation.GetProfileDisplayStrings(item, gui.State.SpansProjects.Profiles)
		},
		FlexColumns: func() []utils.FlexColumn {
			return []utils.FlexColumn{{
				Index:    projectColumns(gui.State.SpansProjects.Profiles) + presentation.ProfileDescriptionColumn,
				MinWidth: presentation.MinProfileDescriptionWidth,
			}}
		},
	}
}

func (gui *Gui) renderProfileDevices(profile *commands.Profile) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		if len(profile.Profile.Devices) == 0 {
			return gui.Tr.NoDevices
		}

		table, err := utils.RenderTable(presentation.GetDeviceRows(profile.Profile.Devices))
		if err != nil {
			return err.Error()
		}

		return table
	})
}

func (gui *Gui) renderProfileConfig(profile *commands.Profile) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string { return gui.profileConfigStr(profile) })
}

func (gui *Gui) profileConfigStr(profile *commands.Profile) string {
	padding := identityPadding
	output := gui.locationStr(gui.sessionLocation(profile.Profile.Project))
	output += utils.WithPadding("Name: ", padding) + profile.Name + "\n"
	output += utils.WithPadding("Used by: ", padding) + fmt.Sprint(profile.UsedByCount()) + "\n"

	data, err := utils.MarshalIntoYaml(profile.Profile)
	if err != nil {
		return fmt.Sprintf("Error marshalling profile details: %v", err)
	}

	output += fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))

	return output
}

func (gui *Gui) fetchProfiles() (func() error, error) {
	ticket := gui.refreshes.profiles.issue()

	profiles, err := gui.IncusCommand.GetProfiles()
	if err != nil {
		return nil, err
	}

	return func() error {
		if !gui.refreshes.profiles.admit(ticket) {
			return nil
		}

		gui.State.SpansProjects.Profiles = spansMultipleProjects(
			lo.Map(profiles, func(profile *commands.Profile, _ int) string { return profile.Profile.Project }))

		gui.Panels.Profiles.SetItems(profiles)

		return gui.Panels.Profiles.RerenderList()
	}, nil
}

func (gui *Gui) refreshProfiles() error {
	return gui.refresh(nil, gui.fetchProfiles)
}

func (gui *Gui) refreshProfilesQuiet() error {
	if err := gui.refreshProfiles(); err != nil {
		gui.Log.Warn().Err(err).Send()
	}

	return nil
}

func (gui *Gui) showProfileUsers(profile *commands.Profile) error {
	return gui.showUsers(profile.Name, profile.IsUsedBy)
}

func (gui *Gui) profileEdit(profile *commands.Profile) error {
	return gui.editInIncus(profile.Profile.Project, []fetch{gui.fetchProfiles}, "profile", "edit", profile.Name)
}

// profileDelete says what the daemon would refuse - any project's default,
// a profile in use - rather than asking first.
func (gui *Gui) profileDelete(profile *commands.Profile) error {
	if profile.Name == defaultProfile {
		return gui.createErrorPanel(gui.Tr.CannotDeleteDefaultProfile)
	}

	name := gui.qualified(profile.Name, profile.Profile.Project, gui.State.SpansProjects.Profiles)
	if profile.UsedByCount() > 0 {
		return gui.refuseInUse(fmt.Sprintf(gui.Tr.ProfileNamed, name), profile.UsedByCount())
	}

	prompt := fmt.Sprintf(gui.Tr.DeleteProfile, name)

	return gui.createConfirmationPanel(gui.Tr.Confirm, prompt, func(g *gocui.Gui, v *gocui.View) error {
		return gui.WithWaitingStatus(gui.Tr.RemovingStatus, func() error {
			if err := profile.Delete(); err != nil {
				return gui.createErrorPanel(err.Error())
			}

			return gui.refreshProfiles()
		})
	}, nil)
}
