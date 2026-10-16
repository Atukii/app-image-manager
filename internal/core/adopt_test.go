package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/testkit"
)

func TestAdoptIdentifiesTheVersionByItsDigestAndTakesTheFileOver(t *testing.T) {
	w := newWorld(t)
	old := w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	downloaded := w.download("obsidian.AppImage", old["Obsidian-1.9.0.AppImage"])

	if _, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.Yes); err != nil {
		t.Fatal(err)
	}

	apps, _ := w.core().List(context.Background())
	if len(apps) != 1 || apps[0].ID != "obsidian" || apps[0].Version != "v1.9.0" ||
		exists(downloaded) || !exists(w.path("Applications/obsidian/obsidian.AppImage")) {
		t.Fatalf("got %+v; want obsidian v1.9.0 managed, moved out of Downloads into its App Folder", apps)
	}
}

func TestAFailedAdoptPutsTheUsersFileBack(t *testing.T) {
	w := newWorld(t)
	old := w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})
	downloaded := w.download("obsidian.AppImage", old["Obsidian-1.9.0.AppImage"])
	w.systemd.FailEnable = errors.New("Failed to enable unit: Access denied")

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.Yes)

	if data, _ := os.ReadFile(downloaded); err == nil || string(data) != string(old["Obsidian-1.9.0.AppImage"]) {
		t.Fatalf("got %v; want an error and the user's AppImage back in Downloads, unchanged", err)
	}
}

// unknownObsidian puts an Obsidian AppImage in Downloads that matches no
// published Release, as an old or self-built download would.
func unknownObsidian(t *testing.T, w *world) string {
	t.Helper()
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	return w.download("Obsidian-1.8.2.AppImage", []byte(obsidianImageBytes(t)))
}

func TestAdoptAsksAboutAnUncertainVersionWithoutChoosingForTheUser(t *testing.T) {
	w := newWorld(t)
	downloaded := unknownObsidian(t, w)

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.Yes)

	q := needAnswers(t, err)
	if q.ID != "adopt.version" || q.Default != nil || q.YesOK ||
		!strings.Contains(q.Prompt, "1.8.2") || !strings.Contains(q.Prompt, "filename") || !exists(downloaded) {
		t.Fatalf("got %+v; want the version Question showing the filename's 1.8.2, with no default, and nothing moved", q)
	}
}

func TestAnAdoptedAppWithAGivenVersionUpdatesNormally(t *testing.T) {
	w := newWorld(t)
	downloaded := unknownObsidian(t, w)
	ctx := context.Background()

	if _, err := w.core().Adopt(ctx, downloaded, obsidianURL, answers(core.Fixed{"adopt.version": {Value: "v1.8.2"}})); err != nil {
		t.Fatal(err)
	}
	listed, _ := w.core().List(ctx)
	results, _ := w.core().Check(ctx, "obsidian")

	if listed[0].Version != "v1.8.2" || results[0].Result != "installed" || results[0].Tag != "v1.10.0" {
		t.Fatalf("listed %+v, check %+v; want v1.8.2 recorded, then v1.10.0 installed", listed[0], results)
	}
}

func TestAdoptCanReplaceTheFileWithTheLatestRelease(t *testing.T) {
	w := newWorld(t)
	downloaded := unknownObsidian(t, w)
	ctx := context.Background()

	if _, err := w.core().Adopt(ctx, downloaded, obsidianURL, answers(core.Fixed{"adopt.version": {Value: "latest"}})); err != nil {
		t.Fatal(err)
	}

	apps, _ := w.core().List(ctx)
	if apps[0].Version != "v1.10.0" || exists(downloaded) || !strings.Contains(desktopEntry(t, w), "MimeType=") {
		t.Fatalf("got %+v; want v1.10.0 installed in place of the old file", apps[0])
	}
}

// handMadeEntry writes the Desktop Entry a user made for an AppImage before
// AiM, and returns its path.
func handMadeEntry(t *testing.T, w *world, appImage string) string {
	t.Helper()
	path := w.path(".local/share/applications/my-obsidian.desktop")
	os.MkdirAll(filepath.Dir(path), 0o755)
	entry := "[Desktop Entry]\nType=Application\nName=My Obsidian\n" +
		"Exec=env GDK_SCALE=2 " + appImage + " --disable-gpu %U\nCategories=Utility;\n"
	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// knownObsidian puts the published Obsidian v1.9.0 in Downloads.
func knownObsidian(t *testing.T, w *world) string {
	t.Helper()
	old := w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})
	return w.download("Obsidian-1.9.0.AppImage", old["Obsidian-1.9.0.AppImage"])
}

func TestAdoptShowsBothValuesOfEachDifferingDesktopEntryField(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	handMadeEntry(t, w, downloaded)

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.With(core.Fixed{"app-id": {Value: "obsidian"}}))

	q := needAnswers(t, err)
	want := []core.Option{{Key: "a", Label: `"My Obsidian"`}, {Key: "b", Label: `"Obsidian"`}}
	if q.ID != "consolidate.Name" || !reflect.DeepEqual(q.Options, want) {
		t.Fatalf("got %+v; want Name: a) \"My Obsidian\" b) \"Obsidian\"", q)
	}
}

func TestAdoptBuildsTheEntryFromTheChosenValuesAndTheUsersLaunchSettings(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	handMadeEntry(t, w, downloaded)

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, answers(core.Fixed{
		"consolidate.Name":       {Value: "b"},
		"consolidate.Categories": {Value: "c"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	entry := desktopEntry(t, w)
	for _, want := range []string{
		"Name=Obsidian\n",
		"Categories=Utility;Office;\n",
		"Exec=env GDK_SCALE=2 " + w.path("Applications/obsidian/obsidian.AppImage") + " --disable-gpu --no-sandbox %U\n",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("entry lacks %q:\n%s", want, entry)
		}
	}
}

// consolidated answers the Consolidation of handMadeEntry with Obsidian's
// Embedded Desktop Entry.
var consolidated = core.Fixed{"consolidate.Name": {Value: "a"}, "consolidate.Categories": {Value: "b"}}

func TestAdoptDeletesTheUsersOldDesktopEntryByDefault(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	old := handMadeEntry(t, w, downloaded)

	if _, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, answers(consolidated)); err != nil {
		t.Fatal(err)
	}

	if exists(old) {
		t.Fatal("want the old Desktop Entry deleted, so the app isn't listed twice")
	}
}

func TestAdoptCanKeepTheUsersOldDesktopEntry(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	old := handMadeEntry(t, w, downloaded)

	if _, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, answers(core.Fixed{
		"consolidate.Name": consolidated["consolidate.Name"], "consolidate.Categories": consolidated["consolidate.Categories"],
		"adopt.delete-old": {Value: "no"},
	})); err != nil {
		t.Fatal(err)
	}

	if !exists(old) {
		t.Fatal("want the old Desktop Entry kept, as asked")
	}
}

func TestAdoptShowsTheFinalDesktopEntryBeforeWritingIt(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	handMadeEntry(t, w, downloaded)

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.With(core.Fixed{
		"app-id":                 {Value: "obsidian"},
		"consolidate.Name":       {Value: "a"},
		"consolidate.Categories": {Value: "b"},
		"adopt.delete-old":       {Value: "yes"},
	}))

	q := needAnswers(t, err)
	plan := strings.Join(q.Plan, "\n")
	if q.ID != "confirm" || !strings.Contains(plan, "Name=My Obsidian\n") || !strings.Contains(plan, "Categories=Office;\n") {
		t.Fatalf("got %+v; want the confirm Question previewing the final entry", q)
	}
}

func TestAdoptingAnAppImageWithoutAnyDesktopEntryAsksForAReviewOfTheDraft(t *testing.T) {
	w := newWorld(t)
	tool := w.publish("v2.0", oct1, map[string]testkit.AppImage{"MyTool-2.0.AppImage": toolImage})
	downloaded := w.download("MyTool-2.0.AppImage", tool["MyTool-2.0.AppImage"])

	_, err := w.core().Adopt(context.Background(), downloaded, toolURL, core.With(core.Fixed{"app-id": {Value: "my-tool"}}))

	if q := needAnswers(t, err); q.ID != "entry.Name" {
		t.Fatalf("got %+v; want the drafted entry's review", q)
	}
}

func TestAFirstAdoptSaysItInstalledTheTimers(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)

	out, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.Yes)
	if err != nil {
		t.Fatal(err)
	}

	if len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "systemd") {
		t.Fatalf("notes %q; want the note about installing the systemd timers", out.Notes)
	}
}

func TestConsolidationChoicesAreNeverMadeForTheUser(t *testing.T) {
	w := newWorld(t)
	downloaded := knownObsidian(t, w)
	handMadeEntry(t, w, downloaded)

	_, err := w.core().Adopt(context.Background(), downloaded, obsidianURL, core.Yes)

	if q := needAnswers(t, err); q.ID != "consolidate.Name" || q.Default != nil || q.YesOK {
		t.Fatalf("got %+v; want the Name choice with no default, which --yes can't answer", q)
	}
}
