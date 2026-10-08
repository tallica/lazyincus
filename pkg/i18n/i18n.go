package i18n

import (
	"github.com/rs/zerolog"
)

// Localizer will translate a message into the user's language
type Localizer struct {
	Log *zerolog.Logger
	S   TranslationSet
}

// NewTranslationSetFromConfig builds a translation set. For this MVP we only
// support English, so the configured language is currently ignored beyond
// logging it.
func NewTranslationSetFromConfig(log *zerolog.Logger, configLanguage string) (*TranslationSet, error) {
	return NewTranslationSet(log, configLanguage), nil
}

func NewTranslationSet(log *zerolog.Logger, language string) *TranslationSet {
	log.Info().Msg("language: " + language)

	baseSet := englishSet()

	return &baseSet
}
