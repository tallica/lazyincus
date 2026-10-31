package presentation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// MinImageLabelWidth is as narrow as the label goes to make room for the
// columns after it. Cached images carry descriptions like "Alpine edge
// arm64 (20260911_13:02)", which is informative well before its last word.
const MinImageLabelWidth = 28

// GetImageDisplayStrings renders one row of the images panel. The users
// count comes straight after the label, where a narrow panel still shows
// it: it's what says whether the image can go. newer is marked ahead of the
// label, which a narrow panel cuts from the end.
func GetImageDisplayStrings(image *commands.Image, showProject bool, newer bool) []string {
	label := image.Label()
	if newer {
		label = utils.ColoredString("↑", color.FgYellow) + " " + label
	}

	cells := []string{
		label,
		displayImageUsers(image),
		utils.ColoredString(image.ShortFingerprint(), color.FgCyan),
		utils.ColoredString(displayImageSize(image), color.FgYellow),
		displayImageLastUsed(image),
		displayImageFlags(image),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(image.Image.Project, color.FgCyan)}, cells...)
	}

	return cells
}

// displayImageUsers marks the images nothing was created from, the ones a
// prune would take.
func displayImageUsers(image *commands.Image) string {
	if image.UsersUnknown {
		return "?"
	}

	count := strconv.Itoa(len(image.UsedBy))
	if image.IsUnused() {
		return utils.ColoredString(count, color.FgRed)
	}

	return utils.ColoredString(count, color.FgGreen)
}

// displayImageLastUsed is when an instance was last created from the image,
// the date alone: an image's staleness is counted in days.
func displayImageLastUsed(image *commands.Image) string {
	if image.Image.LastUsedAt.IsZero() || image.Image.LastUsedAt.Unix() <= 0 {
		return "never"
	}

	return image.Image.LastUsedAt.Local().Format(DateFormat)
}

// displayImageFlags keeps to what sets an image apart, a container image
// being the norm: a VM image, and one Incus cached on a launch and will
// expire by itself.
func displayImageFlags(image *commands.Image) string {
	flags := []string{}
	if image.IsVM() {
		flags = append(flags, "vm")
	}

	if image.Image.Cached {
		flags = append(flags, "cached")
	}

	if len(flags) == 0 {
		return ""
	}

	return utils.ColoredString(strings.Join(flags, " "), color.FgMagenta)
}

func displayImageSize(image *commands.Image) string {
	if image.Image.Size <= 0 {
		return ""
	}

	return units.GetByteSizeStringIEC(image.Image.Size, 2)
}

// ImageConfig is the image's Config tab, upstream its ImageUpstream lines.
func ImageConfig(image *commands.Image, loc Location, upstream string, tr *i18n.TranslationSet) string {
	output := LocationLines(loc)
	output += utils.WithPadding("Alias: ", IdentityPadding) + image.Alias() + "\n"
	output += utils.WithPadding("Fingerprint: ", IdentityPadding) + image.Fingerprint + "\n"
	output += utils.WithPadding("Type: ", IdentityPadding) + image.Image.Type + "\n"
	output += utils.WithPadding("Architecture: ", IdentityPadding) + image.Image.Architecture + "\n"
	output += utils.WithPadding("Uploaded: ", IdentityPadding) + image.Image.UploadedAt.String() + "\n"
	output += utils.WithPadding("Last used: ", IdentityPadding) + image.Image.LastUsedAt.String() + "\n"
	output += utils.WithPadding("Cached: ", IdentityPadding) + fmt.Sprint(image.Image.Cached) + "\n"
	output += utils.WithPadding("Used by: ", IdentityPadding) + ImageUsers(image, tr) + "\n"
	output += upstream

	data, err := utils.MarshalIntoYaml(image.Image)
	if err != nil {
		return fmt.Sprintf("Error marshalling image details: %v", err)
	}

	return output + fmt.Sprintf("\nFull details:\n\n%s", utils.ColoredYamlString(string(data)))
}

// ImageUsers names the instances, grouped by project, which is how they
// read in the instances panel.
func ImageUsers(image *commands.Image, tr *i18n.TranslationSet) string {
	if image.UsersUnknown {
		return tr.ImageUsersUnknown
	}

	if image.IsUnused() {
		return tr.UsedByNothing
	}

	byProject := lo.GroupBy(image.UsedBy, func(user string) string {
		project, _, _ := strings.Cut(user, "/")
		return project
	})

	projects := lo.Keys(byProject)
	slices.Sort(projects)

	groups := lo.Map(projects, func(project string, _ int) string {
		names := lo.Map(byProject[project], func(user string, _ int) string {
			_, name, _ := strings.Cut(user, "/")
			return name
		})

		return fmt.Sprintf("%s (%s)", strings.Join(names, ", "), project)
	})

	return strings.Join(groups, "; ")
}

// ImageUpstream is the image's registry check for its Config tab, empty
// when none has been made.
func ImageUpstream(status commands.UpstreamStatus, tr *i18n.TranslationSet) string {
	if status.State == 0 {
		return ""
	}

	return utils.WithPadding("Upstream: ", IdentityPadding) + status.Ref + "\n" +
		utils.WithPadding("Registry: ", IdentityPadding) + UpstreamVerdict(status, tr) + "\n" +
		utils.WithPadding("Checked: ", IdentityPadding) + status.CheckedAt.Local().Format(time.DateTime) + "\n"
}

// UpstreamVerdict is what a check found, empty for none.
func UpstreamVerdict(status commands.UpstreamStatus, tr *i18n.TranslationSet) string {
	switch status.State {
	case commands.UpstreamNewer:
		return tr.UpstreamNewer
	case commands.UpstreamCurrent:
		return tr.UpstreamCurrent
	case commands.UpstreamFailed:
		return status.Err.Error()
	}

	return ""
}
