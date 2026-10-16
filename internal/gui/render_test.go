package gui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"appimage-manager/internal/core"
	"appimage-manager/internal/state"
)

// TestRender draws the window with sample apps to PNGs in $AIM_GUI_RENDER,
// for reviewing the design. It is skipped unless that is set.
func TestRender(t *testing.T) {
	dir := os.Getenv("AIM_GUI_RENDER")
	if dir == "" {
		t.Skip("set AIM_GUI_RENDER to a directory to render screenshots")
	}
	icons := t.TempDir()
	icon := func(name string, c color.NRGBA) string {
		img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		for y := 4; y < 60; y++ {
			for x := 4; x < 60; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
		path := filepath.Join(icons, name+".png")
		f, _ := os.Create(path)
		png.Encode(f, img)
		f.Close()
		return path
	}
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	apps := []core.AppStatus{
		{ID: "bitwarden", Name: "Bitwarden (work)", Version: "v2025.9.1", Poll: state.Weekly, Skipped: "v2025.10.0",
			LastCheck: &state.LastCheck{At: at, Result: "skipped"}, Source: "https://github.com/bitwarden/clients",
			AssetPattern: "Bitwarden-*-x86_64.AppImage", DesktopFile: "bitwarden.desktop", Icon: icon("bitwarden", color.NRGBA{0x17, 0x5d, 0xdc, 255})},
		{ID: "joplin", Name: "Joplin", Version: "v3.0.15", Poll: state.Daily,
			LastCheck: &state.LastCheck{At: at, Result: "failed", Error: "GitHub API rate limit exceeded", RateLimited: true},
			Source:    "https://github.com/laurent22/joplin", AssetPattern: "Joplin-*.AppImage", DesktopFile: "joplin.desktop", Icon: icon("joplin", color.NRGBA{0x2b, 0x6c, 0xc4, 255})},
		{ID: "krita", Name: "Krita", Version: "v5.2.9", Poll: state.Never, Source: "https://download.kde.org/stable/krita/5.2.9/krita-5.2.9-x86_64.appimage",
			AssetPattern: "krita-5.2.9-x86_64.appimage", DesktopFile: "org.kde.krita.desktop", Icon: icon("krita", color.NRGBA{0xc4, 0x52, 0x1b, 255})},
		{ID: "neovim", Name: "Neovim", Version: "v0.12.6", Previous: "v0.12.5", Poll: state.Daily, LastCheck: &state.LastCheck{At: at, Result: "installed"},
			Source: "https://github.com/neovim/neovim", AssetPattern: "nvim-linux-x86_64.appimage", DesktopFile: "neovim.desktop", Icon: icon("neovim", color.NRGBA{0x3c, 0x8e, 0x3e, 255})},
		{ID: "obsidian", Name: "Obsidian", Version: "v1.10.0", Previous: "v1.9.0", Poll: state.Weekly, Reopen: true,
			LastCheck: &state.LastCheck{At: at, Result: "installed"}, Source: "https://github.com/obsidianmd/obsidian-releases",
			AssetPattern: "Obsidian-*.AppImage", DesktopFile: "md.obsidian.Obsidian.desktop", Icon: icon("obsidian", color.NRGBA{0x6b, 0x4f, 0xbb, 255})},
	}
	apps[4].Overrides.Args = []string{"--ozone-platform=wayland"}
	found := []core.Found{
		{Path: "/home/jakub/Downloads/Obsidian-1.8.2.AppImage", SourceURL: "https://github.com/obsidianmd/obsidian-releases"},
		{Path: "/home/jakub/Downloads/LocalSend-1.15.4-linux-x86-64.AppImage", SourceURL: "https://github.com/localsend/localsend"},
	}

	for _, v := range []struct {
		name    string
		variant fyne.ThemeVariant
	}{{"dark", theme.VariantDark}, {"light", theme.VariantLight}} {
		variant := v.variant
		forcedVariant = &variant
		a := test.NewTempApp(t)
		u := New(a, NewClient(nil))
		u.win.Resize(fyne.NewSize(1040, 680))
		u.apps, u.found = apps, found
		u.render()
		save(t, u.win, filepath.Join(dir, v.name+"-activity.png"))
		u.openID = "obsidian"
		u.tabs.SelectIndex(1)
		u.renderApps()
		u.split.offset = 0
		save(t, u.win, filepath.Join(dir, v.name+"-apps-panel.png"))
		u.closePanel()
		save(t, u.win, filepath.Join(dir, v.name+"-apps.png"))

		def := "obsidian"
		u.ask("Add an app", "", []core.Question{{
			ID: "app-id", Prompt: "App ID (names its folder and files; it can't be changed later without removing and re-adding the app)",
			Default: &def, YesOK: true, Basis: "b",
			Facts: map[string]string{"file": "Obsidian-1.10.0.AppImage", "version": "v1.10.0", "size": "121.3 MB", "verified": "yes"},
		}}, func(core.Fixed) {})
		save(t, u.win, filepath.Join(dir, v.name+"-dialog-appid.png"))
		u.win.Canvas().Overlays().Remove(u.win.Canvas().Overlays().Top())

		u.ask("Add Obsidian-1.8.2.AppImage", "", []core.Question{
			{ID: "consolidate.Name", Basis: "n", Options: []core.Option{{Key: "a", Label: `"My Obsidian"`}, {Key: "b", Label: `"Obsidian"`}}},
			{ID: "consolidate.Categories", Basis: "c", Options: []core.Option{{Key: "a", Label: `"Utility;"`}, {Key: "b", Label: `"Office;"`}, {Key: "c", Label: `both: "Utility;Office;"`}}},
		}, func(core.Fixed) {})
		save(t, u.win, filepath.Join(dir, v.name+"-dialog-consolidate.png"))
		u.win.Canvas().Overlays().Remove(u.win.Canvas().Overlays().Top())
	}
	forcedVariant = nil
}

func save(t *testing.T, w fyne.Window, path string) {
	t.Helper()
	img := w.Canvas().Capture()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
