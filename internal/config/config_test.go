package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"appimage-manager/internal/config"
	"appimage-manager/internal/state"
)

func noEnv(string) string { return "" }

func TestAMissingFileGivesTheDefaults(t *testing.T) {
	got, err := config.Load(filepath.Join(t.TempDir(), "config.toml"), noEnv)
	if err != nil {
		t.Fatal(err)
	}

	want := config.Config{DefaultPoll: state.Weekly, Notify: config.Notify{Installed: true, Failed: true, DigestMismatch: true}}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestLoadReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("default_poll = \"daily\"\ngithub_token = \"ghp_file\"\n\n[notify]\ninstalled = false\n"), 0o600)

	got, err := config.Load(path, noEnv)
	if err != nil {
		t.Fatal(err)
	}

	if got.DefaultPoll != state.Daily || got.GitHubToken != "ghp_file" || got.Notify.Installed || !got.Notify.Failed {
		t.Fatalf("got %+v; want daily, the file's token, installed notifications off and the rest on", got)
	}
}

func TestTheGitHubTokenVariableTakesPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("github_token = \"ghp_file\"\n"), 0o600)

	got, _ := config.Load(path, func(k string) string { return map[string]string{"GITHUB_TOKEN": "ghp_env"}[k] })

	if got.GitHubToken != "ghp_env" {
		t.Fatalf("got %q; want the GITHUB_TOKEN variable's ghp_env", got.GitHubToken)
	}
}

func TestLoadRejectsAnUnknownPollRate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("default_poll = \"hourly\"\n"), 0o600)

	if _, err := config.Load(path, noEnv); err == nil {
		t.Fatal("want hourly refused")
	}
}
