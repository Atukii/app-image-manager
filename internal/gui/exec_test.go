package gui_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"appimage-manager/internal/gui"
)

// TestTheClientDrivesTheRealAim builds aim and talks to it through the
// real process, in an empty home directory.
func TestTheClientDrivesTheRealAim(t *testing.T) {
	if testing.Short() {
		t.Skip("builds aim")
	}
	bin := filepath.Join(t.TempDir(), "aim")
	if out, err := exec.Command("go", "build", "-o", bin, "appimage-manager/cmd/aim").CombinedOutput(); err != nil {
		t.Fatalf("building aim: %v\n%s", err, out)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "GITHUB_TOKEN"} {
		t.Setenv(v, "")
	}
	client := gui.NewClient(gui.Exec{Path: bin})
	ctx := context.Background()

	apps, err := client.List(ctx)
	if err != nil || len(apps) != 0 {
		t.Fatalf("list: %+v, %v; want no apps yet", apps, err)
	}
	if err := client.SetConfig(ctx, "default_poll", "monthly"); err != nil {
		t.Fatal(err)
	}
	settings, err := client.Config(ctx)
	if err != nil || settings.DefaultPoll != "monthly" || settings.GitHubTokenSet {
		t.Fatalf("config: %+v, %v; want monthly and no token", settings, err)
	}
	if err := client.Simple(ctx, "rollback", "nothing-here"); err == nil {
		t.Fatal("want aim's error for an unknown app passed through")
	}
}
