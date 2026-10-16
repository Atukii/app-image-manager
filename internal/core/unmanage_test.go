package core_test

import (
	"context"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/testkit"
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

// stagedObsidian adds Obsidian v1.9.0 and stages v1.10.0 while it runs,
// returning the function that stops it.
func stagedObsidian(t *testing.T, w *world) (stop func()) {
	t.Helper()
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	stop = w.start("obsidian")
	if results, _ := w.core().Check(context.Background(), "obsidian"); results[0].Result != "staged" {
		t.Fatalf("setup: got %+v", results)
	}
	return stop
}

func TestUnmanageDiscardsAStagedUpdateByDefault(t *testing.T) {
	w := newWorld(t)
	stagedObsidian(t, w)()

	if _, err := w.core().Unmanage(context.Background(), "obsidian", core.Yes); err != nil {
		t.Fatal(err)
	}

	if exists(w.path(".local/share/aim/staging/obsidian")) {
		t.Fatal("want the Staged Update discarded")
	}
}

func TestUnmanageCanInstallTheStagedUpdateFirst(t *testing.T) {
	w := newWorld(t)
	stagedObsidian(t, w)()

	_, err := w.core().Unmanage(context.Background(), "obsidian", answers(core.Fixed{"unmanage.staged": {Value: "install"}}))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(desktopEntry(t, w), "MimeType=") { // only v1.10's entry has a MimeType
		t.Fatal("want v1.10.0 installed before AiM let go")
	}
}

func TestUnmanageOnlyOffersToInstallTheStagedUpdateWhenTheAppIsClosed(t *testing.T) {
	w := newWorld(t)
	stagedObsidian(t, w) // still running

	_, err := w.core().Unmanage(context.Background(), "obsidian")

	q := needAnswers(t, err)
	for _, o := range q.Options {
		if o.Key == "install" {
			t.Fatalf("got options %+v; want no install option while obsidian runs", q.Options)
		}
	}
	if q.ID != "unmanage.staged" {
		t.Fatalf("got %+v; want the staged Question", q)
	}
}

func TestInstallingTheStagedUpdateOnUnmanageAsksAboutTheVersionItReplaces(t *testing.T) {
	w := newWorld(t)
	stagedObsidian(t, w)()

	_, err := w.core().Unmanage(context.Background(), "obsidian", core.With(core.Fixed{"unmanage.staged": {Value: "install"}}))

	if q := needAnswers(t, err); q.ID != "unmanage.previous" || !strings.Contains(q.Prompt, "v1.9.0") {
		t.Fatalf("got %+v; want to be asked whether to keep v1.9.0", q)
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
