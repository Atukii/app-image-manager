package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestConfigSetAndShow(t *testing.T) {
	f := newFixture(t)

	for _, kv := range [][2]string{{"default_poll", "daily"}, {"github_token", "github_pat_example"}, {"notify.failed", "false"}} {
		if code, _, stderr := f.run("", "config", "set", kv[0], kv[1]); code != 0 {
			t.Fatalf("set %s: exit %d: %s", kv[0], code, stderr)
		}
	}

	_, stdout, _ := f.run("", "config", "--json")
	var got struct {
		Result struct {
			DefaultPoll   string `json:"default_poll"`
			GitHubToken   bool   `json:"github_token_set"`
			NotifyFailed  bool   `json:"notify.failed"`
			TrayAutostart bool   `json:"tray_autostart"`
		}
	}
	json.Unmarshal([]byte(lastLine(stdout)), &got)
	r := got.Result
	if r.DefaultPoll != "daily" || !r.GitHubToken || r.NotifyFailed || r.TrayAutostart {
		t.Fatalf("got %+v from %s", r, stdout)
	}
	if info, err := os.Stat(f.settings.File); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.toml: %v; want it readable only by its owner, since it holds the token", err)
	}
}

func TestConfigNeverPrintsTheToken(t *testing.T) {
	f := newFixture(t)
	f.run("", "config", "set", "github_token", "github_pat_secret")

	_, text, _ := f.run("", "config")
	_, js, _ := f.run("", "config", "--json")

	if strings.Contains(text+js, "github_pat_secret") {
		t.Fatal("want the token never printed, only whether one is set")
	}
}

func TestTrayAutostartWritesAndRemovesTheLoginEntry(t *testing.T) {
	f := newFixture(t)

	f.run("", "config", "set", "tray_autostart", "true")
	entry, err := os.ReadFile(f.settings.Autostart)
	if err != nil || !strings.Contains(string(entry), "Exec=/home/jakub/.local/bin/aim-gui --tray") {
		t.Fatalf("got %q (%v); want an autostart entry running aim-gui --tray", entry, err)
	}

	f.run("", "config", "set", "tray_autostart", "false")
	if _, err := os.Stat(f.settings.Autostart); err == nil {
		t.Fatal("want the autostart entry removed")
	}
}

func TestConfigRefusesUnknownKeysAndValues(t *testing.T) {
	f := newFixture(t)

	for _, kv := range [][2]string{{"colour", "violet"}, {"default_poll", "hourly"}, {"notify.failed", "maybe"}} {
		if code, _, _ := f.run("", "config", "set", kv[0], kv[1]); code == 0 {
			t.Errorf("config set %s %s: want it refused", kv[0], kv[1])
		}
	}
}

func TestDismiss(t *testing.T) {
	f := added(t)

	if code, _, stderr := f.run("", "dismiss", "obsidian"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
