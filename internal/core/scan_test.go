package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// download puts an AppImage in ~/Downloads, as a browser would.
func (w *world) download(name string, data []byte) string {
	w.t.Helper()
	path := w.path(filepath.Join("Downloads", name))
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		w.t.Fatal(err)
	}
	return path
}

func foundPaths(t *testing.T, w *world) []string {
	t.Helper()
	found, err := w.core().Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range found {
		paths = append(paths, f.Path)
	}
	return paths
}

func TestScanFindsAppImagesAiMDoesNotManage(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	joplin := w.download("Joplin-3.0.15.AppImage", []byte(obsidianImageBytes(t)))
	w.download("notes.txt", []byte("not an AppImage"))

	got := foundPaths(t, w)

	if len(got) != 1 || got[0] != joplin {
		t.Fatalf("got %q; want only %s (not the managed Obsidian)", got, joplin)
	}
}

func obsidianImageBytes(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(obsidianImage.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestScanSuggestsTheSourceURLFromTheUpdateInformation(t *testing.T) {
	w := newWorld(t)
	withUpdateInfo := obsidianImage
	withUpdateInfo.UpdateInfo = "gh-releases-zsync|obsidianmd|obsidian-releases|latest|Obsidian-*.AppImage.zsync"
	data, _ := os.ReadFile(withUpdateInfo.Build(t))
	w.download("Obsidian-1.9.0.AppImage", data)

	found, _ := w.core().Scan(context.Background())

	if len(found) != 1 || found[0].SourceURL != "https://github.com/obsidianmd/obsidian-releases" {
		t.Fatalf("got %+v; want the GitHub repository suggested", found)
	}
}

func TestAnIgnoredAppImageIsNotOfferedAgain(t *testing.T) {
	w := newWorld(t)
	joplin := w.download("Joplin-3.0.15.AppImage", []byte(obsidianImageBytes(t)))

	if err := w.core().Ignore(context.Background(), joplin); err != nil {
		t.Fatal(err)
	}

	if got := foundPaths(t, w); len(got) != 0 {
		t.Fatalf("got %q; want the ignored AppImage left out", got)
	}
}
