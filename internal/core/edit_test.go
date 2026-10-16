package core_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/state"
	"appimage-manager/internal/testkit"
)

func desktopEntry(t *testing.T, w *world) string {
	t.Helper()
	data, err := os.ReadFile(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditRewritesTheDesktopEntryAtOnce(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	err := w.core().Edit(context.Background(), "obsidian", core.Edit{Set: map[string]string{"Name": "My Notes"}})
	if err != nil {
		t.Fatal(err)
	}

	if entry := desktopEntry(t, w); !strings.Contains(entry, "Name=My Notes\n") {
		t.Fatalf("got:\n%s\nwant Name=My Notes", entry)
	}
}

func TestAnOverrideSurvivesAnUpdate(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.core().Edit(ctx, "obsidian", core.Edit{Set: map[string]string{"Name": "My Notes"}})
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	w.core().Check(ctx, "obsidian")

	if entry := desktopEntry(t, w); !strings.Contains(entry, "Name=My Notes\n") || !strings.Contains(entry, "MimeType=") {
		t.Fatalf("got:\n%s\nwant v1.10's entry with Name=My Notes kept", entry)
	}
}

func TestResettingAFieldBringsBackTheEmbeddedValue(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.core().Edit(ctx, "obsidian", core.Edit{Set: map[string]string{"Name": "My Notes"}})

	if err := w.core().Edit(ctx, "obsidian", core.Edit{Reset: []string{"Name"}}); err != nil {
		t.Fatal(err)
	}

	if entry := desktopEntry(t, w); !strings.Contains(entry, "Name=Obsidian\n") {
		t.Fatalf("got:\n%s\nwant the embedded Name=Obsidian back", entry)
	}
}

func TestEditSetsLaunchArgumentsAndEnvironment(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	err := w.core().Edit(context.Background(), "obsidian", core.Edit{
		Args: &[]string{"--ozone-platform=wayland"},
		Env:  &[]string{"ELECTRON_OZONE_PLATFORM_HINT=wayland"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "Exec=env ELECTRON_OZONE_PLATFORM_HINT=wayland " + w.path("Applications/obsidian/obsidian.AppImage") + " --ozone-platform=wayland --no-sandbox %U\n"
	if entry := desktopEntry(t, w); !strings.Contains(entry, want) {
		t.Fatalf("got:\n%s\nwant %q", entry, want)
	}
}

// myIcon writes an SVG icon the user picked and returns its path.
func myIcon(t *testing.T, w *world) string {
	t.Helper()
	path := w.path("Pictures/notes.svg")
	os.MkdirAll(w.path("Pictures"), 0o755)
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><!-- mine --></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnIconChosenWithEditSurvivesAnUpdate(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	if err := w.core().Edit(ctx, "obsidian", core.Edit{Icon: myIcon(t, w)}); err != nil {
		t.Fatal(err)
	}
	w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})

	w.core().Check(ctx, "obsidian")

	icon, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.svg"))
	entry := desktopEntry(t, w)
	if !strings.Contains(string(icon), "mine") || !strings.Contains(entry, "Icon="+w.path("Applications/obsidian/obsidian.svg")+"\n") {
		t.Fatalf("icon %q, entry:\n%s\nwant the user's SVG icon kept and used", icon, entry)
	}
}

func TestResettingTheIconBringsBackTheAppImagesOwn(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.core().Edit(ctx, "obsidian", core.Edit{Icon: myIcon(t, w)})

	if err := w.core().Edit(ctx, "obsidian", core.Edit{ResetIcon: true}); err != nil {
		t.Fatal(err)
	}

	icon, _ := os.ReadFile(w.path("Applications/obsidian/obsidian.png"))
	_, svgErr := os.Stat(w.path("Applications/obsidian/obsidian.svg"))
	if string(icon) != "\x89PNG\r\n\x1a\nobsidian-icon" || svgErr == nil || !strings.Contains(desktopEntry(t, w), "obsidian.png\n") {
		t.Fatalf("png %q, svg left %v; want the embedded PNG back and in use", icon, svgErr == nil)
	}
}

func TestEditRefusesFieldsAiMManagesItself(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	before := desktopEntry(t, w)

	err := w.core().Edit(context.Background(), "obsidian", core.Edit{Set: map[string]string{"Exec": "/usr/bin/false"}})

	if err == nil || !strings.Contains(err.Error(), "Exec") || desktopEntry(t, w) != before {
		t.Fatalf("got %v; want Exec refused and the entry unchanged", err)
	}
}

func TestListShowsTheOverrides(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	w.core().Edit(ctx, "obsidian", core.Edit{Set: map[string]string{"Name": "My Notes"}, Args: &[]string{"--ozone-platform=wayland"}})

	apps, _ := w.core().List(ctx)

	if apps[0].Overrides.Fields["Name"] != "My Notes" || len(apps[0].Overrides.Args) != 1 || apps[0].Name != "My Notes" {
		t.Fatalf("got %+v; want the Name and argument Overrides shown", apps[0])
	}
}

func TestSetPollReschedulesOnlyThatApp(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()

	if err := w.core().SetPoll(ctx, "obsidian", state.Daily); err != nil {
		t.Fatal(err)
	}

	dropIn, _ := os.ReadFile(w.path(".config/systemd/user/aim-check@obsidian.timer.d/poll.conf"))
	apps, _ := w.core().List(ctx)
	if !strings.Contains(string(dropIn), "OnCalendar=daily") || apps[0].Poll != state.Daily {
		t.Fatalf("drop-in %q, listed %q; want daily", dropIn, apps[0].Poll)
	}
}

func TestSetPollNeverRemovesTheAppsTimer(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	if err := w.core().SetPoll(context.Background(), "obsidian", state.Never); err != nil {
		t.Fatal(err)
	}

	if w.systemd.Enabled["aim-check@obsidian.timer"] {
		t.Fatal("want obsidian's timer gone")
	}
}

func TestSetPollRefusesUnknownRates(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	if err := w.core().SetPoll(context.Background(), "obsidian", "hourly"); err == nil {
		t.Fatal("want hourly refused: the Poll Rates are daily, weekly, monthly and never")
	}
}

func TestAddCanSetThePollRate(t *testing.T) {
	w := newWorld(t)
	w.publish("v1.9.0", oct1, map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})

	if _, err := w.core().Add(context.Background(), obsidianURL, core.Yes, core.Poll(state.Monthly)); err != nil {
		t.Fatal(err)
	}

	apps, _ := w.core().List(context.Background())
	if apps[0].Poll != state.Monthly {
		t.Fatalf("got %q; want monthly", apps[0].Poll)
	}
}
