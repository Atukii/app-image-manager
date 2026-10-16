package core_test

import (
	"context"
	"strings"
	"testing"

	"appimage-manager/internal/core"
)

func TestUnmanageLeavesTheAppWorkingWithoutAiM(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()

	if _, err := w.core().Unmanage(ctx, "obsidian", core.Yes); err != nil {
		t.Fatal(err)
	}

	entry := desktopEntry(t, w)
	apps, _ := w.core().List(ctx)
	if !exists(w.path("Applications/obsidian/obsidian.AppImage")) || strings.Contains(entry, "X-AiM-") ||
		!strings.Contains(entry, "Exec="+w.path("Applications/obsidian/obsidian.AppImage")) ||
		len(apps) != 0 || len(w.systemd.Enabled) != 0 || exists(w.path(".config/systemd/user/aim-check@.timer")) {
		t.Fatalf("entry:\n%s\napps %v, timers %v; want the app in place, unmarked, untracked and unscheduled", entry, apps, w.systemd.Enabled)
	}
}

func TestUnmanageDeletesThePreviousVersionByDefault(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)

	if _, err := w.core().Unmanage(context.Background(), "obsidian", core.Yes); err != nil {
		t.Fatal(err)
	}

	if exists(w.path("Applications/obsidian/obsidian.AppImage.previous")) {
		t.Fatal("want the Previous Version deleted")
	}
}

func TestUnmanageCanKeepThePreviousVersion(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)

	_, err := w.core().Unmanage(context.Background(), "obsidian", answers(core.Fixed{"unmanage.previous": {Value: "keep"}}))
	if err != nil {
		t.Fatal(err)
	}

	if !exists(w.path("Applications/obsidian/obsidian.AppImage.previous")) {
		t.Fatal("want the Previous Version kept")
	}
}

func TestUnmanageCanMoveTheAppToTheStandardLayout(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	_, err := w.core().Unmanage(context.Background(), "obsidian", answers(core.Fixed{"unmanage.layout": {Value: "standard"}}))
	if err != nil {
		t.Fatal(err)
	}

	appImage := w.path("Applications/Obsidian.AppImage")
	icon := w.path(".local/share/icons/hicolor/256x256/apps/obsidian.png")
	entry := desktopEntry(t, w)
	if !exists(appImage) || !exists(icon) || exists(w.path("Applications/obsidian")) ||
		!strings.Contains(entry, "Exec="+appImage+" --no-sandbox %U\n") || !strings.Contains(entry, "Icon="+icon+"\n") {
		t.Fatalf("entry:\n%s\nwant the AppImage and icon in the standard places, the entry pointing at them, and the App Folder gone", entry)
	}
}
