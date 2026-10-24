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

// A stack with an instance from an older config says so after its status.
func TestGetStackDisplayStringsChanged(t *testing.T) {
	stack := &commands.ComposeStack{
		Dir:      "/srv/web",
		Name:     "web",
		Services: []commands.ComposeService{{Name: "web", Hash: "v1:new"}},
		Statuses: map[string][]string{"web": {"Running"}},
		Hashes:   map[string][]string{"web": {"v1:new"}},
	}

	assert.Equal(t, []string{"web", "running", "/srv/web"}, GetStackDisplayStrings(&config.GuiConfig{}, stack, "/home", nil, nil))

	stack.Hashes["web"] = []string{"v1:old"}
	assert.Equal(t, []string{"web", "running changed", "/srv/web"}, GetStackDisplayStrings(&config.GuiConfig{}, stack, "/home", nil, nil))
}
