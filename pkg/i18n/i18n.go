package i18n

import (
	"github.com/rs/zerolog"
)

func NewTranslationSet(log *zerolog.Logger, language string) *TranslationSet {
	log.Info().Msg("language: " + language)

	baseSet := englishSet()

	return &baseSet
}
