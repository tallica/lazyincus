package commands

import (
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tallica/lazyincus/pkg/commands/incustest"
	"github.com/tallica/lazyincus/pkg/i18n"
)

func imagesServer() *incustest.Server {
	return incustest.New(incustest.Server{
		Instances: []api.InstanceFull{{Instance: api.Instance{
			Name: "web", Project: "stack",
			InstancePut: api.InstancePut{Config: map[string]string{"volatile.base_image": "used"}},
		}}},
		Images: []api.Image{
			{Fingerprint: "used", Project: "default"},
			{Fingerprint: "spare", Project: "default"},
		},
	})
}

func getImages(t *testing.T, server *incustest.Server) map[string]*Image {
	t.Helper()

	log := NewDummyLog()
	command := NewIncusCommandWithClient(log, NewDummyOSCommand(), i18n.NewTranslationSet(log, "en"),
		NewDummyAppConfig(), server, "fake")

	images, err := command.GetImages()
	require.NoError(t, err)

	byFingerprint := map[string]*Image{}
	for _, image := range images {
		byFingerprint[image.Fingerprint] = image
	}

	return byFingerprint
}

func TestImagesKnowTheirUsers(t *testing.T) {
	images := getImages(t, imagesServer())

	assert.Equal(t, []string{"stack/web"}, images["used"].UsedBy)
	assert.False(t, images["used"].IsUnused())
	assert.True(t, images["spare"].IsUnused())
}

// Instances that can't be listed leave the images listed, with nothing
// taken for unused - prune goes by it.
func TestImagesWithoutTheirUsers(t *testing.T) {
	server := imagesServer()
	server.SetDown(true)

	images := getImages(t, server)

	require.Len(t, images, 2)
	for _, image := range images {
		assert.True(t, image.UsersUnknown)
		assert.False(t, image.IsUnused())
	}
}
