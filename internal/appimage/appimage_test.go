package appimage_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"appimage-manager/internal/appimage"
)

const obsidianEntry = "[Desktop Entry]\nName=Obsidian\nExec=AppRun %U\nIcon=obsidian\nType=Application\n"

func TestInspectFindsTheEmbeddedDesktopEntryAndItsFilename(t *testing.T) {
	path := fixture{files: map[string]string{
		"md.obsidian.Obsidian.desktop": obsidianEntry,
		"AppRun":                       "#!/bin/sh\n",
	}}.build(t)

	info, err := appimage.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}

	if info.DesktopFile != "md.obsidian.Obsidian.desktop" || string(info.DesktopEntry) != obsidianEntry {
		t.Fatalf("got %q with entry %q", info.DesktopFile, info.DesktopEntry)
	}
}

// png is the 8-byte PNG signature followed by filler; enough to be
// recognised as a PNG.
const png = "\x89PNG\r\n\x1a\nfake-image-data"

func TestInspectFollowsTheDirIconSymlinksToTheIcon(t *testing.T) {
	path := fixture{
		files: map[string]string{"usr/share/icons/hicolor/256x256/apps/obsidian.png": png},
		symlinks: map[string]string{
			".DirIcon":     "obsidian.png",
			"obsidian.png": "usr/share/icons/hicolor/256x256/apps/obsidian.png",
		},
	}.build(t)

	info, err := appimage.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(info.Icon) != png || info.IconExt != ".png" {
		t.Fatalf("got icon %q with extension %q; want the PNG and \".png\"", info.Icon, info.IconExt)
	}
}

func TestInspectReadsTheEmbeddedUpdateInformation(t *testing.T) {
	const updateInfo = "gh-releases-zsync|obsidianmd|obsidian-releases|latest|Obsidian-*.AppImage.zsync"
	path := fixture{files: map[string]string{"AppRun": "#!/bin/sh\n"}, updateInfo: updateInfo}.build(t)

	info, err := appimage.Inspect(path)
	if err != nil {
		t.Fatal(err)
	}

	if info.UpdateInfo != updateInfo {
		t.Fatalf("got %q; want %q", info.UpdateInfo, updateInfo)
	}
}

func TestInspectRejectsFilesThatAreNotType2AppImages(t *testing.T) {
	type1 := append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 'A', 'I', 1}, make([]byte, 100)...)
	for name, content := range map[string][]byte{
		"type-1 AppImage": type1,
		"shell script":    []byte("#!/bin/sh\necho hello\n"),
	} {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, content, 0o755); err != nil {
			t.Fatal(err)
		}

		if _, err := appimage.Inspect(path); !errors.Is(err, appimage.ErrNotAppImage) {
			t.Errorf("%s: got error %v; want ErrNotAppImage", name, err)
		}
	}
}
