package presentation

import (
	"github.com/fatih/color"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// GetImageDisplayStrings renders one row of the images panel, roughly
// `incus image list`'s columns minus the ones that are the same for every
// local image.
func GetImageDisplayStrings(image *commands.Image) []string {
	return []string{
		displayImageAlias(image),
		utils.ColoredString(image.ShortFingerprint(), color.FgCyan),
		utils.ColoredString(displayImageType(image), color.FgMagenta),
		utils.ColoredString(displayImageSize(image), color.FgYellow),
	}
}

func displayImageAlias(image *commands.Image) string {
	alias := image.Alias()
	if alias == "" {
		// Images pulled in as a dependency have no alias; the fingerprint
		// column still identifies them.
		return utils.ColoredString("(no alias)", color.FgBlue)
	}

	return alias
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
