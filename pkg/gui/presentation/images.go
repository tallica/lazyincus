package presentation

import (
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// MinImageLabelWidth is as narrow as the label goes to make room for the
// columns after it. Cached images carry descriptions like "Alpine edge
// arm64 (20260911_13:02)", which is informative well before its last word.
const MinImageLabelWidth = 28

// GetImageDisplayStrings renders one row of the images panel. The users
// count comes straight after the label, where a narrow panel still shows
// it: it's what says whether the image can go.
func GetImageDisplayStrings(image *commands.Image, showProject bool) []string {
	cells := []string{
		image.Label(),
		displayImageUsers(image),
		utils.ColoredString(image.ShortFingerprint(), color.FgCyan),
		utils.ColoredString(displayImageSize(image), color.FgYellow),
		displayImageLastUsed(image),
		utils.ColoredString(displayImageFlags(image), color.FgMagenta),
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

	return strings.Join(flags, " ")
}

func displayImageSize(image *commands.Image) string {
	if image.Image.Size <= 0 {
		return ""
	}

	return units.GetByteSizeStringIEC(image.Image.Size, 2)
}
