package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"
)

// History names one of the typed-command histories.
type History int

const (
	IncusHistory History = iota
	ComposeHistory
)

const historyLength = 100

// commandHistory is history.yml, beside state.yml: the command lines run
// from : and ;, oldest first.
type commandHistory struct {
	Incus   []string `yaml:"incus,omitempty"`
	Compose []string `yaml:"compose,omitempty"`
}

func (h *commandHistory) lines(history History) *[]string {
	if history == ComposeHistory {
		return &h.Compose
	}

	return &h.Incus
}

// HistoryFilename is where the command history lives.
func (c *AppConfig) HistoryFilename() string {
	return filepath.Join(c.ConfigDir, "history.yml")
}

func (c *AppConfig) loadHistory() (*commandHistory, error) {
	saved := &commandHistory{}

	content, err := os.ReadFile(c.HistoryFilename())
	if errors.Is(err, os.ErrNotExist) {
		return saved, nil
	}

	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, saved); err != nil {
		return nil, fmt.Errorf("%s: %w", c.HistoryFilename(), err)
	}

	return saved, nil
}

// LoadHistory is history's lines, oldest first; a missing file is none.
func (c *AppConfig) LoadHistory(history History) ([]string, error) {
	saved, err := c.loadHistory()
	if err != nil {
		return nil, err
	}

	return *saved.lines(history), nil
}

// RecordCommand saves line as history's newest, dropping an earlier copy of
// it and the oldest past historyLength. Like state.yml, the file is re-read
// first, so another session's lines aren't lost.
func (c *AppConfig) RecordCommand(history History, line string) error {
	saved, err := c.loadHistory()
	if err != nil {
		return err
	}

	lines := saved.lines(history)
	*lines = slices.DeleteFunc(*lines, func(kept string) bool { return kept == line })
	*lines = append(*lines, line)

	if over := len(*lines) - historyLength; over > 0 {
		*lines = slices.Delete(*lines, 0, over)
	}

	content, err := yaml.Marshal(saved)
	if err != nil {
		return err
	}

	return writeFileAtomically(c.HistoryFilename(), content)
}
