package core_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"appimage-manager/internal/core"
	"appimage-manager/internal/source"
	"appimage-manager/internal/testkit"
)

// world is a complete fake environment for core: a temp home directory, a
// Release Source with published Releases, a file server for their Assets
// and a fake systemd.
type world struct {
	t       *testing.T
	home    string
	systemd *testkit.Systemd
	files   map[string][]byte // asset path on the file server → content
	server  *httptest.Server
	source  *fakeSource
	now     time.Time
	running map[string]bool // AppImage paths currently running
	// refreshed records each directory the desktop menu was refreshed for.
	refreshed []string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		t:       t,
		home:    t.TempDir(),
		files:   map[string][]byte{},
		source:  &fakeSource{},
		now:     time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
		running: map[string]bool{},
	}
	w.systemd = testkit.NewSystemd(w.path(".config/systemd/user"))
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		data, ok := w.files[r.URL.Path]
		if !ok {
			http.NotFound(rw, r)
			return
		}
		rw.Write(data)
	}))
	t.Cleanup(w.server.Close)
	return w
}

// path is a path inside the fake home directory.
func (w *world) path(rel string) string { return filepath.Join(w.home, rel) }

func (w *world) core() *core.Core {
	paths := core.HomePaths(w.home)
	paths.SystemApplications = []string{w.path("usr/share/applications")}
	return core.New(core.Deps{
		Paths:     paths,
		HTTP:      w.server.Client(),
		Now:       func() time.Time { return w.now },
		ProcRoot:  w.path("proc"),
		Sources:   func(source.Ref) (source.ReleaseSource, error) { return w.source, nil },
		Systemctl: w.systemd,
		Aim:       "/home/jakub/.local/bin/aim",
		RefreshMenu: func(dir string) {
			w.refreshed = append(w.refreshed, dir)
		},
	})
}

// publish adds a Release whose Assets are the given AppImages, served with
// their Digests, and returns their contents by name.
func (w *world) publish(tag string, published time.Time, assets map[string]testkit.AppImage) map[string][]byte {
	w.t.Helper()
	rel := source.Release{Tag: tag, Published: published}
	contents := map[string][]byte{}
	for name, img := range assets {
		data, err := os.ReadFile(img.Build(w.t))
		if err != nil {
			w.t.Fatal(err)
		}
		urlPath := "/download/" + tag + "/" + name
		w.files[urlPath] = data
		sum := sha256.Sum256(data)
		rel.Assets = append(rel.Assets, source.Asset{
			Name: name, URL: w.server.URL + urlPath, Size: int64(len(data)),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
		})
		contents[name] = data
	}
	w.source.releases = append([]source.Release{rel}, w.source.releases...)
	return contents
}

// fakeSource is a Release Source whose Releases the test publishes.
type fakeSource struct {
	releases []source.Release // newest first
}

func (f *fakeSource) Latest(context.Context) (source.Release, error) {
	if len(f.releases) == 0 {
		return source.Release{}, os.ErrNotExist
	}
	return f.releases[0], nil
}

func (f *fakeSource) Recent(context.Context) ([]source.Release, error) { return f.releases, nil }

// obsidianImage is a fake Obsidian AppImage with an Embedded Desktop Entry
// and icon.
var obsidianImage = testkit.AppImage{
	Files: map[string]string{
		"md.obsidian.Obsidian.desktop": "[Desktop Entry]\nName=Obsidian\nExec=AppRun --no-sandbox %U\nIcon=obsidian\nType=Application\nCategories=Office;\n",
		"obsidian.png":                 "\x89PNG\r\n\x1a\nobsidian-icon",
		"AppRun":                       "#!/bin/sh\n",
	},
	Symlinks: map[string]string{".DirIcon": "obsidian.png"},
}

const obsidianURL = "https://github.com/obsidianmd/obsidian-releases"
