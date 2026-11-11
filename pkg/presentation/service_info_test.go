package presentation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
	"github.com/tallica/lazyincus/pkg/utils"
)

// Each replica's block says whether up would recreate it, or that it
// predates the hash; a lone instance's change is the service's line.
func TestServiceInfoConfig(t *testing.T) {
	hashed := func(name, hash string) *commands.Instance {
		return composeInstance(name, "web", map[string]string{"user.label.incus-compose.config-hash": hash})
	}

	tr := i18n.NewTranslationSet(commands.NewDummyLog(), "en")
	info := func(service *commands.ComposeService) string {
		row := &commands.ServiceRow{Service: service}
		noInfo := func(*commands.Instance, ...string) string { return "" }

		return utils.Decolorise(ServiceInfo(row, Location{}, "", nil, noInfo, 40, tr))
	}

	replicas := info(&commands.ComposeService{Name: "web", Hash: "v1:new", Replicas: 3, Instances: []*commands.Instance{
		hashed("web-1", "v1:old"), hashed("web-2", "v1:new"), composeInstance("web-3", "web", nil),
	}})
	assert.Contains(t, replicas, "Changed:      1 instance from an older config, recreate with u\n")
	assert.Contains(t, replicas, "Config:       changed, recreate with u\n")
	assert.Contains(t, replicas, "Config:       not tracked, created before incus-compose 1.4.0\n")
	assert.Equal(t, 2, strings.Count(replicas, "\nConfig:"))

	lone := info(&commands.ComposeService{Name: "web", Hash: "v1:new", Instances: []*commands.Instance{hashed("web", "v1:old")}})
	assert.Contains(t, lone, "Changed:")
	assert.Zero(t, strings.Count(lone, "\nConfig:"))

	unhashed := info(&commands.ComposeService{Name: "web", Instances: []*commands.Instance{
		composeInstance("web-1", "web", nil), composeInstance("web-2", "web", nil),
	}})
	assert.Zero(t, strings.Count(unhashed, "\nConfig:"))
}
