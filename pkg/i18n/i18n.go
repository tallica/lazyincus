package i18n

import (
	"github.com/sirupsen/logrus"
)

// Localizer will translate a message into the user's language
type Localizer struct {
	Log *logrus.Entry
	S   TranslationSet
}

// NewTranslationSetFromConfig builds a translation set. For this MVP we only
// support English, so the configured language is currently ignored beyond
// logging it.
func NewTranslationSetFromConfig(log *logrus.Entry, configLanguage string) (*TranslationSet, error) {
	return NewTranslationSet(log, configLanguage), nil
}

func NewTranslationSet(log *logrus.Entry, language string) *TranslationSet {
	log.Info("language: " + language)

	baseSet := englishSet()

	return &baseSet
}
