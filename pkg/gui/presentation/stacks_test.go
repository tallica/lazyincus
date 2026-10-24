package presentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// A stack running an image whose tag has a newer build says so by its name,
// whatever its other images say.
func TestStackImageMarker(t *testing.T) {
	stack := &commands.ComposeStack{Dir: "/srv/web", Name: "web", Images: []string{"current", "old"}}
	updates := map[string]commands.UpstreamStatus{
		"current": {State: commands.UpstreamCurrent},
		"old":     {State: commands.UpstreamNewer},
	}

	assert.Equal(t, "web ↑", utils.Decolorise(GetStackDisplayStrings(&config.GuiConfig{}, stack, "", nil, updates)[0]))

	stack.Images = []string{"current"}
	assert.Equal(t, "web", utils.Decolorise(GetStackDisplayStrings(&config.GuiConfig{}, stack, "", nil, updates)[0]))
}
