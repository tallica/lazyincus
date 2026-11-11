package presentation

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// A stack running an image whose tag has a newer build says so after its
// status, whatever its other images say.
func TestStackImageMarker(t *testing.T) {
	stack := &commands.ComposeStack{Dir: "/srv/web", Name: "web", Images: []string{"current", "old"}}
	updates := map[string]commands.UpstreamStatus{
		"current": {State: commands.UpstreamCurrent},
		"old":     {State: commands.UpstreamNewer},
	}

	stack.Services = []commands.ComposeService{{Name: "web", Hash: "v1:new"}}
	stack.Statuses = map[string][]string{"web": {"Running"}}
	stack.Hashes = map[string][]string{"web": {"v1:old"}}
	row := func() []string {
		return lo.Map(GetStackDisplayStrings(&config.GuiConfig{}, stack, "", nil, updates)[:2],
			func(cell string, _ int) string { return utils.Decolorise(cell) })
	}

	assert.Equal(t, []string{"web", "running ↻ ↑"}, row())

	stack.Images = []string{"current"}
	assert.Equal(t, []string{"web", "running ↻"}, row())
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
	assert.Equal(t, []string{"web", "running ↻", "/srv/web"}, GetStackDisplayStrings(&config.GuiConfig{}, stack, "/home", nil, nil))
}
