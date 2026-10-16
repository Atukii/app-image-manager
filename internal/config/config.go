// Package config reads AiM's settings from config.toml (CFG-1).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/BurntSushi/toml"

	"appimage-manager/internal/state"
)

// Config is AiM's settings.
type Config struct {
	DefaultPoll state.PollRate `toml:"default_poll"`
	// GitHubToken raises GitHub's API rate limit; GITHUB_TOKEN overrides it.
	GitHubToken string `toml:"github_token"`
	Notify      Notify `toml:"notify"`
}

// Notify turns each kind of notification on or off.
type Notify struct {
	Installed      bool `toml:"installed"`
	Staged         bool `toml:"staged"`
	Failed         bool `toml:"failed"`
	DigestMismatch bool `toml:"digest_mismatch"`
}

// Load reads the config file at path; a missing file gives the defaults.
// getenv reads environment variables.
func Load(path string, getenv func(string) string) (Config, error) {
	c := Config{DefaultPoll: state.Weekly, Notify: Notify{Installed: true, Staged: true, Failed: true, DigestMismatch: true}}
	if _, err := toml.DecodeFile(path, &c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if !slices.Contains([]state.PollRate{state.Daily, state.Weekly, state.Monthly, state.Never}, c.DefaultPoll) {
		return Config{}, fmt.Errorf("%s: default_poll %q: want daily, weekly, monthly or never", path, c.DefaultPoll)
	}
	if token := getenv("GITHUB_TOKEN"); token != "" {
		c.GitHubToken = token
	}
	return c, nil
}

// Wants reports whether notifications of kind are on.
func (n Notify) Wants(kind string) bool {
	switch kind {
	case "installed":
		return n.Installed
	case "staged":
		return n.Staged
	case "failed":
		return n.Failed
	case "digest-mismatch":
		return n.DigestMismatch
	}
	return true
}
