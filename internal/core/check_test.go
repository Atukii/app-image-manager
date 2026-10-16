package core_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"appimage-manager/internal/testkit"
)

var oct10 = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

// obsidian110 is the next Obsidian release's AppImage.
var obsidian110 = testkit.AppImage{Files: map[string]string{
	"md.obsidian.Obsidian.desktop": "[Desktop Entry]\nName=Obsidian\nExec=AppRun --no-sandbox %U\nType=Application\nCategories=Office;\nMimeType=x-scheme-handler/obsidian;\n",
	"obsidian.png":                 "\x89PNG\r\n\x1a\nobsidian-icon-1.10",
	"AppRun":                       "#!/bin/sh\n# 1.10\n",
}, Symlinks: map[string]string{".DirIcon": "obsidian.png"}}

func TestCheckInstallsANewerRelease(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	contents := w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	results, err := w.core().Check(context.Background(), "obsidian")
	if err != nil {
		t.Fatal(err)
	}

	installed, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	if len(results) != 1 || results[0].Result != "installed" || string(installed) != string(contents["Obsidian-1.10.0.AppImage"]) {
		t.Fatalf("got %+v; want v1.10.0 installed", results)
	}
}

func TestCheckRecordsTheNewVersionSoTheNextCheckIsUpToDate(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	ctx := context.Background()
	w.core().Check(ctx, "obsidian")

	again, _ := w.core().Check(ctx, "obsidian")
	apps, _ := w.core().List(ctx)

	if again[0].Result != "up-to-date" || apps[0].Version != "v1.10.0" {
		t.Fatalf("second check %+v, listed version %q; want up-to-date at v1.10.0", again, apps[0].Version)
	}
}

func TestASwapKeepsThePreviousVersion(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	before, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	w.core().Check(context.Background(), "obsidian")

	previous, err := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage.previous"))
	if err != nil || string(previous) != string(before) {
		t.Fatalf("want v1.9.0's AppImage kept as obsidian.AppImage.previous (err %v)", err)
	}
}

func TestCheckNotifiesWhenItInstallsAnUpdate(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	w.core().Check(context.Background(), "obsidian")

	if len(w.notified) != 1 || w.notified[0].Kind != "installed" || !strings.Contains(w.notified[0].Summary, "v1.10.0") {
		t.Fatalf("notified %+v; want one installed notification naming v1.10.0", w.notified)
	}
}

func TestASwapRebuildsTheDesktopEntryAndIconFromTheNewAppImage(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	w.core().Check(context.Background(), "obsidian")

	entry, _ := os.ReadFile(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))
	icon, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.png"))
	if !strings.Contains(string(entry), "MimeType=x-scheme-handler/obsidian;\n") || string(icon) != "\x89PNG\r\n\x1a\nobsidian-icon-1.10" {
		t.Fatalf("entry:\n%s\nicon %q; want v1.10's MimeType and icon", entry, icon)
	}
}

func TestCheckRetriesThreeTimesThenNotifiesTheFailure(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.source.Failures = 4

	results, _ := w.core().Check(context.Background(), "obsidian")

	wantSleeps := []time.Duration{10 * time.Second, 30 * time.Second, 90 * time.Second}
	if results[0].Result != "failed" || !reflect.DeepEqual(w.slept, wantSleeps) ||
		len(w.notified) != 1 || w.notified[0].Kind != "failed" {
		t.Fatalf("got %+v, slept %v, notified %+v; want 3 retries after 10s/30s/90s and one failed notification",
			results, w.slept, w.notified)
	}
}

func TestCheckSucceedsSilentlyWhenARetryWorks(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.source.Failures = 1

	results, _ := w.core().Check(context.Background(), "obsidian")

	if results[0].Result != "installed" || len(w.notified) != 1 || w.notified[0].Kind != "installed" {
		t.Fatalf("got %+v, notified %+v; want installed and only the installed notification", results, w.notified)
	}
}

func TestADigestMismatchIsNotRetriedAndIsNotifiedAtOnce(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.tamper("v1.10.0", "Obsidian-1.10.0.AppImage")

	results, _ := w.core().Check(context.Background(), "obsidian")

	apps, _ := w.core().List(context.Background())
	if results[0].Result != "failed" || len(w.slept) != 0 || apps[0].Version != "v1.9.0" ||
		len(w.notified) != 1 || w.notified[0].Kind != "digest-mismatch" {
		t.Fatalf("got %+v, slept %v, notified %+v; want no retry, v1.9.0 kept and one digest-mismatch notification",
			results, w.slept, w.notified)
	}
}

func TestARejectedReleaseIsNotDownloadedOrReportedAgain(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.tamper("v1.10.0", "Obsidian-1.10.0.AppImage")
	ctx := context.Background()
	w.core().Check(ctx, "obsidian")
	downloads, notified := w.source.Downloads, len(w.notified)

	w.core().Check(ctx, "obsidian")

	if w.source.Downloads != downloads || len(w.notified) != notified {
		t.Fatalf("%d more downloads, %d more notifications; want none for the rejected v1.10.0",
			w.source.Downloads-downloads, len(w.notified)-notified)
	}
}

func TestADigestMismatchOnARetryIsStillReportedAsAMismatch(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.tamper("v1.10.0", "Obsidian-1.10.0.AppImage")
	w.source.Failures = 1

	w.core().Check(context.Background(), "obsidian")

	if len(w.notified) != 1 || w.notified[0].Kind != "digest-mismatch" {
		t.Fatalf("notified %+v; want one digest-mismatch notification", w.notified)
	}
}

func TestCheckingEveryAppRecordsEachResultForList(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.start("obsidian")

	if _, err := w.core().Check(ctx); err != nil {
		t.Fatal(err)
	}

	apps, _ := w.core().List(ctx)
	if apps[0].LastCheck == nil || !apps[0].LastCheck.At.Equal(w.now) || apps[0].LastCheck.Result != "installed" {
		t.Fatalf("got %+v; want the installed result of the Check at %s", apps[0], w.now)
	}
}

func TestCheckInstallsTheUpdateEvenWhileTheAppIsRunning(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	contents := w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.start("obsidian")

	results, _ := w.core().Check(context.Background(), "obsidian")

	installed, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage"))
	if results[0].Result != "installed" || !results[0].Reopen || string(installed) != string(contents["Obsidian-1.10.0.AppImage"]) {
		t.Fatalf("got %+v; want v1.10.0 installed while Obsidian runs, needing a reopen (ADR 0009)", results)
	}
	if len(w.notified) != 1 || w.notified[0].Kind != "installed" || !strings.Contains(w.notified[0].Summary, "reopen") {
		t.Fatalf("notified %+v; want one installed notification asking to reopen Obsidian", w.notified)
	}
}

func TestListSaysReopenWhileTheOldVersionStillRuns(t *testing.T) {
	w := newWorld(t)
	updatedObsidian(t, w)
	stop := w.startReplaced("obsidian")
	ctx := context.Background()

	apps, _ := w.core().List(ctx)
	if !apps[0].Reopen {
		t.Fatalf("got %+v; want Reopen while v1.9.0 still runs", apps[0])
	}

	stop()
	if apps, _ := w.core().List(ctx); apps[0].Reopen {
		t.Fatal("want Reopen cleared once the old version stopped")
	}
}
