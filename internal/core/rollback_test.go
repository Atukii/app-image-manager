package core_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/testkit"
)

// updatedObsidian adds Obsidian v1.9.0 and updates it to v1.10.0, returning
// v1.9.0's AppImage bytes.
func updatedObsidian(t *testing.T, w *world) []byte {
	t.Helper()
	addObsidian(t, w, obsidianImage)
	before, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	if results, _ := w.core().Check(context.Background(), "obsidian"); results[0].Result != "installed" {
		t.Fatalf("setup: got %+v", results)
	}
	return before
}

func TestRollbackRestoresThePreviousVersion(t *testing.T) {
	w := newWorld(t)
	before := updatedObsidian(t, w)

	if _, err := w.core().Rollback(context.Background(), "obsidian"); err != nil {
		t.Fatal(err)
	}

	installed, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	apps, _ := w.core().List(context.Background())
	if string(installed) != string(before) || apps[0].Version != "v1.9.0" {
		t.Fatalf("listed %q; want v1.9.0's AppImage back and listed", apps[0].Version)
	}
}

func TestAfterRollbackChecksSkipTheReleaseRolledBackFrom(t *testing.T) {
	w := newWorld(t)
	before := updatedObsidian(t, w)
	ctx := context.Background()
	w.core().Rollback(ctx, "obsidian")

	results, _ := w.core().Check(ctx, "obsidian")

	installed, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	apps, _ := w.core().List(ctx)
	if results[0].Result != "skipped" || string(installed) != string(before) || apps[0].Skipped != "v1.10.0" {
		t.Fatalf("got %+v, listed %+v; want v1.10.0 skipped and v1.9.0 kept", results, apps[0])
	}
}

func TestANewerReleaseThanTheSkippedOneInstallsAndClearsTheSkip(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)
	ctx := context.Background()
	w.core().Rollback(ctx, "obsidian")
	w.publish("v1.10.1", oct10.AddDate(0, 0, 2), map[string]testkit.AppImage{"Obsidian-1.10.1.AppImage": obsidian110})

	results, _ := w.core().Check(ctx, "obsidian")

	apps, _ := w.core().List(ctx)
	if results[0].Result != "installed" || apps[0].Version != "v1.10.1" || apps[0].Skipped != "" {
		t.Fatalf("got %+v, listed %+v; want v1.10.1 installed and no Skipped Release", results, apps[0])
	}
}

func TestUnskipLetsTheNextCheckInstallTheSkippedRelease(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)
	ctx := context.Background()
	w.core().Rollback(ctx, "obsidian")

	if err := w.core().Unskip(ctx, "obsidian"); err != nil {
		t.Fatal(err)
	}
	results, _ := w.core().Check(ctx, "obsidian")

	if results[0].Result != "installed" || results[0].Tag != "v1.10.0" {
		t.Fatalf("got %+v; want v1.10.0 installed again", results)
	}
}

func TestRollbackWorksWhileTheAppIsRunning(t *testing.T) {
	w := newWorld(t)
	before := updatedObsidian(t, w)
	w.start("obsidian")

	if _, err := w.core().Rollback(context.Background(), "obsidian"); err != nil {
		t.Fatal(err)
	}

	if installed, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage")); string(installed) != string(before) {
		t.Fatal("want v1.9.0 back in place while Obsidian runs (ADR 0009)")
	}
}

func TestAFailedRollbackKeepsTheCurrentVersion(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)
	current, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	os.WriteFile(w.path("Applications/obsidian/obsidian.AppImage.previous"), []byte("corrupted"), 0o755)

	_, err := w.core().Rollback(context.Background(), "obsidian")

	after, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	apps, _ := w.core().List(context.Background())
	if err == nil || string(after) != string(current) || apps[0].Version != "v1.10.0" {
		t.Fatalf("got %v, listed %q; want an error with v1.10.0 still installed", err, apps[0].Version)
	}
}

func TestThereIsNoSecondRollback(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)
	ctx := context.Background()
	w.core().Rollback(ctx, "obsidian")

	_, err := w.core().Rollback(ctx, "obsidian")

	if !errors.Is(err, core.ErrNoPreviousVersion) {
		t.Fatalf("got %v; want ErrNoPreviousVersion (RB-2)", err)
	}
}

func TestSkipStopsChecksInstallingTheLatestRelease(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	ctx := context.Background()

	skipped, err := w.core().Skip(ctx, "obsidian", "")
	if err != nil {
		t.Fatal(err)
	}
	results, _ := w.core().Check(ctx, "obsidian")

	if skipped != "v1.10.0" || results[0].Result != "skipped" {
		t.Fatalf("skipped %q, check %+v; want the latest, v1.10.0, skipped and not installed", skipped, results)
	}
}

func TestSkippingTheInstalledVersionIsRefused(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	_, err := w.core().Skip(context.Background(), "obsidian", "v1.9.0")

	if !errors.Is(err, core.ErrSkipInstalled) {
		t.Fatalf("got %v; want ErrSkipInstalled (that's a rollback)", err)
	}
}
