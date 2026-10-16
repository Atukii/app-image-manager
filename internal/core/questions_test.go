package core_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"appimage-manager/internal/core"
	"appimage-manager/internal/testkit"
)

var oct1 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

// needAnswers returns the Question an operation stopped at.
func needAnswers(t *testing.T, err error) core.Question {
	t.Helper()
	var need *core.NeedAnswers
	if !errors.As(err, &need) {
		t.Fatalf("got error %v; want NeedAnswers", err)
	}
	return need.Questions[0]
}

func answers(fixed core.Fixed) core.Opt {
	return core.With(core.Layered{fixed, core.Defaults})
}

func TestAddAsksWhichAppImageWhenSeveralMatch(t *testing.T) {
	w := newWorld(t)
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{
		"Obsidian-1.9.0.AppImage":           obsidianImage,
		"Obsidian-1.9.0-nosandbox.AppImage": obsidianImage,
	})

	_, err := w.core().Add(context.Background(), obsidianURL, core.Yes)

	q := needAnswers(t, err)
	if q.ID != "asset" || len(q.Options) != 2 || q.Flag != "--asset" {
		t.Fatalf("got %+v; want the asset Question with both AppImages", q)
	}
}

func TestAddInstallsTheChosenAppImage(t *testing.T) {
	w := newWorld(t)
	contents := w.publish("v1.9.0", oct1, map[string]testkit.AppImage{
		"Obsidian-1.9.0.AppImage":           obsidianImage,
		"Obsidian-1.9.0-nosandbox.AppImage": {Files: map[string]string{"AppRun": "#!/bin/sh\n# nosandbox\n"}},
	})

	_, err := w.core().Add(context.Background(), obsidianURL, answers(core.Fixed{"asset": {Value: "Obsidian-1.9.0-nosandbox.AppImage"}}))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.AppImage")); string(got) != string(contents["Obsidian-1.9.0-nosandbox.AppImage"]) {
		t.Fatal("want the chosen AppImage installed")
	}
}

func TestAddRefusesAnAppIDAlreadyInUse(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	_, err := w.core().Add(context.Background(), obsidianURL, core.Yes)

	q := needAnswers(t, err)
	if q.ID != "app-id" || q.Default != nil || !strings.Contains(q.Problem, "already in use") {
		t.Fatalf("got %+v; want the app-id Question with no suggestion and the problem explained", q)
	}
}

func TestWithoutAnswersAddSaysWhichFlagAnswersIt(t *testing.T) {
	w := newWorld(t)
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	_, err := w.core().Add(context.Background(), obsidianURL)

	if q := needAnswers(t, err); q.ID != "app-id" || !strings.Contains(err.Error(), "--app-id") {
		t.Fatalf("got %v; want the app-id Question naming --app-id (CLI-1)", err)
	}
}

// toolImage has no Embedded Desktop Entry.
var toolImage = testkit.AppImage{Files: map[string]string{"AppRun": "#!/bin/sh\n"}}

const toolURL = "https://github.com/acme/my-tool"

func TestAddAsksForAReviewOfTheDraftedEntryBeforeWritingAnything(t *testing.T) {
	w := newWorld(t)
	w.publish("v2.0", oct1, map[string]testkit.AppImage{"MyTool-2.0.AppImage": toolImage})

	_, err := w.core().Add(context.Background(), toolURL, core.With(core.Fixed{"app-id": {Value: "my-tool"}}))

	q := needAnswers(t, err)
	_, statErr := os.Stat(w.path("Applications/my-tool"))
	if q.ID != "entry.Name" || q.Default == nil || *q.Default != "My Tool" || statErr == nil {
		t.Fatalf("got %+v, app folder exists %v; want the drafted Name to review and nothing written", q, statErr == nil)
	}
}

func TestAddWritesTheReviewedDraftEntry(t *testing.T) {
	w := newWorld(t)
	w.publish("v2.0", oct1, map[string]testkit.AppImage{"MyTool-2.0.AppImage": toolImage})

	_, err := w.core().Add(context.Background(), toolURL, answers(core.Fixed{
		"entry.Name":       {Value: "Acme Tool"},
		"entry.Categories": {Value: "Development;"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	entry, _ := os.ReadFile(w.path(".local/share/applications/my-tool.desktop"))
	if !strings.Contains(string(entry), "Name=Acme Tool\n") || !strings.Contains(string(entry), "Categories=Development;\n") {
		t.Fatalf("got:\n%s\nwant the reviewed Name and Categories", entry)
	}
}

// systemEntry puts a Desktop Entry in the fake system applications
// directory, as a distribution package would.
func systemEntry(t *testing.T, w *world, name string) {
	t.Helper()
	dir := w.path("usr/share/applications")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(dir+"/"+name, []byte("[Desktop Entry]\nName=Obsidian (pacman)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAddWarnsBeforeHidingASystemDesktopEntry(t *testing.T) {
	w := newWorld(t)
	systemEntry(t, w, "md.obsidian.Obsidian.desktop")
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	_, err := w.core().Add(context.Background(), obsidianURL, core.With(core.Fixed{"app-id": {Value: "obsidian"}}))

	q := needAnswers(t, err)
	if q.ID != "shadowing" || !strings.Contains(q.Prompt, "/usr/share/applications/md.obsidian.Obsidian.desktop") {
		t.Fatalf("got %+v; want the shadowing Question naming the system entry", q)
	}
}

func TestAddByDefaultAvoidsHidingASystemDesktopEntry(t *testing.T) {
	w := newWorld(t)
	systemEntry(t, w, "md.obsidian.Obsidian.desktop")

	addObsidian(t, w, obsidianImage)

	_, vendorErr := os.Stat(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))
	_, ownErr := os.Stat(w.path(".local/share/applications/obsidian.desktop"))
	if vendorErr == nil || ownErr != nil {
		t.Fatalf("vendor-named entry written %v, obsidian.desktop written %v; want only obsidian.desktop", vendorErr == nil, ownErr == nil)
	}
}

func TestAddUsesTheIconTheUserSupplies(t *testing.T) {
	w := newWorld(t)
	mine := w.path("Pictures/my-obsidian.png")
	os.MkdirAll(w.path("Pictures"), 0o755)
	os.WriteFile(mine, []byte("\x89PNG\r\n\x1a\nmy-own-icon"), 0o644)
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	if _, err := w.core().Add(context.Background(), obsidianURL, core.Yes, core.Icon(mine)); err != nil {
		t.Fatal(err)
	}

	if icon, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.png")); string(icon) != "\x89PNG\r\n\x1a\nmy-own-icon" {
		t.Fatalf("got icon %q; want the user's", icon)
	}
}

func TestAddRefreshesTheDesktopMenu(t *testing.T) {
	w := newWorld(t)

	addObsidian(t, w, obsidianImage)

	if len(w.refreshed) != 1 || w.refreshed[0] != w.path(".local/share/applications") {
		t.Fatalf("refreshed %q; want ~/.local/share/applications once", w.refreshed)
	}
}

func TestOnlyTheFirstAddSaysItInstalledTheTimers(t *testing.T) {
	w := newWorld(t)
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})
	first, err := w.core().Add(context.Background(), obsidianURL, core.Yes)
	if err != nil {
		t.Fatal(err)
	}

	second, err := w.core().Add(context.Background(), obsidianURL, answers(core.Fixed{"app-id": {Value: "obsidian-2"}}))
	if err != nil {
		t.Fatal(err)
	}

	if len(first.Notes) != 1 || !strings.Contains(first.Notes[0], "systemd") || len(second.Notes) != 0 {
		t.Fatalf("first notes %q, second notes %q; want one note about systemd timers, then none", first.Notes, second.Notes)
	}
}
