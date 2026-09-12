package presentation

import (
	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// maxImageLabelWidth keeps a long description from pushing the columns after
// it off the side panel. Cached images carry descriptions like "Alpine edge
// arm64 (20260911_13:02)", which is informative well before its last word.
const maxImageLabelWidth = 28

// GetImageDisplayStrings renders one row of the images panel, roughly
// `incus image list`'s columns minus the ones that are the same for every
// local image.
func GetImageDisplayStrings(image *commands.Image, showProject bool) []string {
	cells := []string{
		utils.Truncate(image.Label(), maxImageLabelWidth),
		utils.ColoredString(image.ShortFingerprint(), color.FgCyan),
		utils.ColoredString(displayImageType(image), color.FgMagenta),
		utils.ColoredString(displayImageSize(image), color.FgYellow),
	}

	if showProject {
		cells = append([]string{utils.ColoredString(image.Image.Project, color.FgCyan)}, cells...)
	}

	return cells
}

func displayImageType(image *commands.Image) string {
	if image.IsVM() {
		return "vm"
	}

	return "container"
}

func displayImageSize(image *commands.Image) string {
	if image.Image.Size <= 0 {
		return ""
	}

	return units.GetByteSizeStringIEC(image.Image.Size, 2)
}
