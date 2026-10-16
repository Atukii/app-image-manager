package core_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"appimage-manager/internal/core"
	"appimage-manager/internal/state"
	"appimage-manager/internal/testkit"
)

func TestAddInstallsTheLatestAppImageIntoItsAppFolder(t *testing.T) {
	w := newWorld(t)
	contents := w.publish("v1.9.0", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	if _, err := w.core().Add(context.Background(), obsidianURL, core.Yes); err != nil {
		t.Fatal(err)
	}

	path := w.path("Applications/obsidian/obsidian.AppImage")
	got, err := os.ReadFile(path)
	info, _ := os.Stat(path)
	if err != nil || string(got) != string(contents["Obsidian-1.9.0.AppImage"]) || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("want the downloaded AppImage, executable, at %s (err %v)", path, err)
	}
}

// addObsidian publishes Obsidian v1.9.0 and adds it, accepting every
// suggestion.
func addObsidian(t *testing.T, w *world, image testkit.AppImage) {
	t.Helper()
	w.publish("v1.9.0", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": image})
	if _, err := w.core().Add(context.Background(), obsidianURL, core.Yes); err != nil {
		t.Fatal(err)
	}
}

func TestAddIntegratesTheAppUnderItsVendorDesktopFilename(t *testing.T) {
	w := newWorld(t)

	addObsidian(t, w, obsidianImage)

	entry, err := os.ReadFile(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Exec=" + w.path("Applications/obsidian/obsidian.AppImage") + " --no-sandbox %U\n",
		"Icon=" + w.path("Applications/obsidian/obsidian.png") + "\n",
		"X-AiM-AppId=obsidian\n",
	} {
		if !strings.Contains(string(entry), want) {
			t.Errorf("Desktop Entry lacks %q:\n%s", want, entry)
		}
	}
	if icon, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.png")); string(icon) != "\x89PNG\r\n\x1a\nobsidian-icon" {
		t.Errorf("got icon %q; want the AppImage's own", icon)
	}
}

func TestAddSchedulesWeeklyChecksByDefault(t *testing.T) {
	w := newWorld(t)

	addObsidian(t, w, obsidianImage)

	dropIn, _ := os.ReadFile(w.path(".config/systemd/user/aim-check@obsidian.timer.d/poll.conf"))
	if !w.systemd.Enabled["aim-check@obsidian.timer"] || !strings.Contains(string(dropIn), "OnCalendar=weekly") {
		t.Fatalf("enabled %v, drop-in %q; want obsidian's timer enabled weekly", w.systemd.Enabled, dropIn)
	}
}

func TestAddUsesTheDefaultIconWhenTheAppImageHasNone(t *testing.T) {
	w := newWorld(t)
	noIcon := testkit.AppImage{Files: map[string]string{"md.obsidian.Obsidian.desktop": obsidianImage.Files["md.obsidian.Obsidian.desktop"]}}

	addObsidian(t, w, noIcon)

	if icon, err := os.ReadFile(w.path("Applications/obsidian/obsidian.svg")); err != nil || !strings.Contains(string(icon), "<svg") {
		t.Fatalf("got %q, %v; want AiM's default SVG icon", icon, err)
	}
}

func TestAddDryRunChangesNothing(t *testing.T) {
	w := newWorld(t)
	w.publish("v1.9.0", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	out, err := w.core().Add(context.Background(), obsidianURL, core.Yes, core.DryRun())
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := os.Stat(w.path("Applications/obsidian"))
	if len(out.Plan) == 0 || statErr == nil || len(w.systemd.Enabled) != 0 {
		t.Fatalf("plan %q, app folder exists %v, timers %v; want a plan and no changes", out.Plan, statErr == nil, w.systemd.Enabled)
	}
}

func TestListShowsTheAddedApp(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	apps, err := w.core().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []core.AppStatus{{ID: "obsidian", Name: "Obsidian", Version: "v1.9.0", Poll: state.Weekly}}
	if !reflect.DeepEqual(apps, want) {
		t.Fatalf("got %+v\nwant %+v", apps, want)
	}
}

func TestAFailedAddLeavesNothingBehind(t *testing.T) {
	w := newWorld(t)
	w.systemd.FailEnable = errors.New("Failed to enable unit: Access denied")
	w.publish("v1.9.0", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	_, err := w.core().Add(context.Background(), obsidianURL, core.Yes)

	apps, _ := w.core().List(context.Background())
	_, folderErr := os.Stat(w.path("Applications/obsidian"))
	_, entryErr := os.Stat(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))
	if err == nil || len(apps) != 0 || folderErr == nil || entryErr == nil {
		t.Fatalf("error %v, apps %v, folder left %v, entry left %v; want the error and nothing left",
			err, apps, folderErr == nil, entryErr == nil)
	}
}
