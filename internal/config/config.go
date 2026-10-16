// Package config reads AiM's settings from config.toml (CFG-1).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"

	"appimage-manager/internal/state"
)

// Config is AiM's settings.
type Config struct {
	DefaultPoll state.PollRate `toml:"default_poll"`
	// GitHubToken raises GitHub's API rate limit; GITHUB_TOKEN overrides it.
	GitHubToken string `toml:"github_token,omitempty"`
	Notify      Notify `toml:"notify"`
}

// Notify turns each kind of notification on or off.
type Notify struct {
	Installed      bool `toml:"installed"`
	Failed         bool `toml:"failed"`
	DigestMismatch bool `toml:"digest_mismatch"`
}

// Load reads the config file at path; a missing file gives the defaults.
// getenv reads environment variables: GITHUB_TOKEN overrides the file's
// token.
func Load(path string, getenv func(string) string) (Config, error) {
	c, err := LoadFile(path)
	if err != nil {
		return Config{}, err
	}
	if token := getenv("GITHUB_TOKEN"); token != "" {
		c.GitHubToken = token
	}
	return c, nil
}

// LoadFile reads only the config file at path, with the defaults for what
// it leaves out; it is what Save writes back.
func LoadFile(path string) (Config, error) {
	c := Config{DefaultPoll: state.Weekly, Notify: Notify{Installed: true, Failed: true, DigestMismatch: true}}
	if _, err := toml.DecodeFile(path, &c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if !ValidPoll(c.DefaultPoll) {
		return Config{}, fmt.Errorf("%s: default_poll %q: want daily, weekly, monthly or never", path, c.DefaultPoll)
	}
	return c, nil
}

// ValidPoll reports whether rate is a Poll Rate.
func ValidPoll(rate state.PollRate) bool {
	return slices.Contains([]state.PollRate{state.Daily, state.Weekly, state.Monthly, state.Never}, rate)
}

// Save writes c to path, readable only by its owner since it may hold a
// GitHub token.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Wants reports whether notifications of kind are on.
func (n Notify) Wants(kind string) bool {
	switch kind {
	case "installed":
		return n.Installed
	case "failed":
		return n.Failed
	case "digest-mismatch":
		return n.DigestMismatch
	}
	return true
}
