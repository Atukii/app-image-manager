package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/testkit"
)

func TestUninstallAlwaysAsksAboutTheConfigEvenWithYes(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	_, err := w.core().Uninstall(context.Background(), core.Yes)

	q := needAnswers(t, err)
	if q.ID != "uninstall.config" || q.Default != nil || q.YesOK || !exists(w.path("Applications/obsidian/obsidian.AppImage")) {
		t.Fatalf("got %+v; want the config Question with no default that --yes can't answer, and nothing changed", q)
	}
}

// aimSettings writes AiM's config file and the tray's autostart entry.
func aimSettings(t *testing.T, w *world) {
	t.Helper()
	for path, content := range map[string]string{
		".config/aim/config.toml":           "default_poll = \"weekly\"\n",
		".config/autostart/aim-gui.desktop": "[Desktop Entry]\nExec=aim-gui --tray\n",
	} {
		os.MkdirAll(filepath.Dir(w.path(path)), 0o755)
		if err := os.WriteFile(w.path(path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUninstallLeavesAppsWorkingAndNoTraceOfAiMButTheKeptConfig(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	aimSettings(t, w)

	if _, err := w.core().Uninstall(context.Background(), answers(core.Fixed{"uninstall.config": {Value: "keep"}})); err != nil {
		t.Fatal(err)
	}

	units, _ := os.ReadDir(w.path(".config/systemd/user"))
	if !exists(w.path("Applications/obsidian/obsidian.AppImage")) || strings.Contains(desktopEntry(t, w), "X-AiM-") ||
		len(units) != 0 || exists(w.path(".local/share/aim")) || exists(w.path(".cache/aim")) ||
		exists(w.path(".config/autostart/aim-gui.desktop")) || !exists(w.path(".config/aim/config.toml")) {
		t.Fatalf("units %v; want the app working and unmarked, no AiM units, data, cache or autostart, and the config kept", units)
	}
}

func TestUninstallDeletesTheConfigWhenAskedTo(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	aimSettings(t, w)

	if _, err := w.core().Uninstall(context.Background(), answers(core.Fixed{"uninstall.config": {Value: "delete"}})); err != nil {
		t.Fatal(err)
	}

	if exists(w.path(".config/aim")) {
		t.Fatal("want ~/.config/aim deleted")
	}
}

// aimBinaries puts aim and aim-gui in ~/.local/bin.
func aimBinaries(t *testing.T, w *world) {
	t.Helper()
	os.MkdirAll(w.path(".local/bin"), 0o755)
	for _, name := range []string{"aim", "aim-gui"} {
		if err := os.WriteFile(w.path(".local/bin/"+name), []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

var keepConfig = core.Fixed{"uninstall.config": {Value: "keep"}}

func TestUninstallDeletesBinariesItCanDelete(t *testing.T) {
	w := newWorld(t)
	aimBinaries(t, w)

	if _, err := w.core().Uninstall(context.Background(), answers(keepConfig)); err != nil {
		t.Fatal(err)
	}

	if exists(w.path(".local/bin/aim")) || exists(w.path(".local/bin/aim-gui")) {
		t.Fatal("want aim and aim-gui deleted from ~/.local/bin")
	}
}

func TestUninstallLeavesPackagedBinariesToPacman(t *testing.T) {
	w := newWorld(t)
	aimBinaries(t, w)
	w.owners[w.path(".local/bin/aim")] = "aim"
	w.owners[w.path(".local/bin/aim-gui")] = "aim-gui"

	out, err := w.core().Uninstall(context.Background(), answers(keepConfig))
	if err != nil {
		t.Fatal(err)
	}

	if !exists(w.path(".local/bin/aim")) || len(out.Notes) != 1 || out.Notes[0] != "Finish with: sudo pacman -R aim aim-gui" {
		t.Fatalf("notes %q; want the binaries left and the pacman command given", out.Notes)
	}
}

func TestUninstallCanAnswerTheAppQuestionsOnceForAllApps(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	addObsidian(t, w, obsidianImage)
	if _, err := w.core().Add(ctx, obsidianURL, answers(core.Fixed{"app-id": {Value: "notes"}})); err != nil {
		t.Fatal(err)
	}
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})
	w.core().Check(ctx) // both apps now have a Previous Version

	_, err := w.core().Uninstall(ctx, answers(core.Fixed{
		"uninstall.config":  {Value: "keep"},
		"uninstall.scope":   {Value: "once"},
		"unmanage.previous": {Value: "keep"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !exists(w.path("Applications/obsidian/obsidian.AppImage.previous")) || !exists(w.path("Applications/notes/notes.AppImage.previous")) {
		t.Fatal("want the one 'keep' answer applied to both apps' Previous Versions")
	}
}

func TestUninstallLeavesNoUnitsEvenWhenNoAppHadATimer(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.core().SetPoll(ctx, "obsidian", "never")

	if _, err := w.core().Uninstall(ctx, answers(keepConfig)); err != nil {
		t.Fatal(err)
	}

	if units, _ := os.ReadDir(w.path(".config/systemd/user")); len(units) != 0 {
		t.Fatalf("got %v; want no AiM units left", units)
	}
}
