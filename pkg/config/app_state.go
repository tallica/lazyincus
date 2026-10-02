package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"
)

// AppState is what lazyincus remembers between sessions: state.yml, beside
// config.yml. The app writes this file and never the config, which is the
// user's own.
type AppState struct {
	// Stacks are the compose project directories added with `a` on the
	// Stacks panel, absolute and cleaned, each after `remote:` for the
	// remote it's pinned to.
	Stacks []string `yaml:"stacks,omitempty"`
}

// ErrStackListed is AddStack refusing a directory already saved.
var ErrStackListed = errors.New("already listed")

// StateFilename is where AppState lives.
func (c *AppConfig) StateFilename() string {
	return filepath.Join(c.ConfigDir, "state.yml")
}

// LoadAppState reads state.yml; a missing one is an empty state.
func (c *AppConfig) LoadAppState() (*AppState, error) {
	state := &AppState{}

	content, err := os.ReadFile(c.StateFilename())
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}

	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(content, state); err != nil {
		return nil, fmt.Errorf("%s: %w", c.StateFilename(), err)
	}

	return state, nil
}

// updateAppState re-reads the file before changing it, so another session's
// write since this one last read isn't lost, and replaces it by rename, so
// a reader never sees half of it.
func (c *AppConfig) updateAppState(change func(*AppState) error) error {
	state, err := c.LoadAppState()
	if err != nil {
		return err
	}

	if err := change(state); err != nil {
		return err
	}

	content, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	return writeFileAtomically(c.StateFilename(), content)
}

func writeFileAtomically(path string, content []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}

	defer os.Remove(file.Name())

	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(file.Name(), path)
}

// AddStack saves a stack, refusing one already saved.
func (c *AppConfig) AddStack(dir string) error {
	return c.updateAppState(func(state *AppState) error {
		if slices.Contains(state.Stacks, dir) {
			return fmt.Errorf("%s: %w", dir, ErrStackListed)
		}

		state.Stacks = append(state.Stacks, dir)

		return nil
	})
}

// RemoveStack forgets a saved stack; one not saved is no error.
func (c *AppConfig) RemoveStack(dir string) error {
	return c.updateAppState(func(state *AppState) error {
		state.Stacks = slices.DeleteFunc(state.Stacks, func(saved string) bool { return saved == dir })

		return nil
	})
}
