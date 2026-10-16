package core_test

import (
	"context"
	"fmt"
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
	source  *testkit.Releases
	now     time.Time
	// slept records each wait between retries.
	slept []time.Duration
	// notified records every notification sent.
	notified []core.Notification
	// owners maps binary paths to the package that installed them.
	owners map[string]string
	// noFuse2 makes fuse2 look missing.
	noFuse2 bool
	// refreshed records each directory the desktop menu was refreshed for.
	refreshed []string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		t:      t,
		home:   t.TempDir(),
		source: testkit.NewReleases(t),
		now:    time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
		owners: map[string]string{},
	}
	w.systemd = testkit.NewSystemd(w.path(".config/systemd/user"))
	return w
}

// path is a path inside the fake home directory.
func (w *world) path(rel string) string { return filepath.Join(w.home, rel) }

func (w *world) core() *core.Core {
	paths := core.HomePaths(w.home)
	paths.SystemApplications = []string{w.path("usr/share/applications")}
	return core.New(core.Deps{
		Paths:     paths,
		HTTP:      w.source.Server.Client(),
		Now:       func() time.Time { return w.now },
		ProcRoot:  w.path("proc"),
		Sources:   func(source.Ref) (source.ReleaseSource, error) { return w.source, nil },
		Systemctl: w.systemd,
		Aim:       "/home/jakub/.local/bin/aim",
		Sleep:     func(d time.Duration) { w.slept = append(w.slept, d) },
		Notifier: notifierFunc(func(n core.Notification) {
			w.notified = append(w.notified, n)
		}),
		Binaries:     []string{w.path(".local/bin/aim"), w.path(".local/bin/aim-gui")},
		PackageOwner: func(path string) string { return w.owners[path] },
		HasFuse2:     func() bool { return !w.noFuse2 },
		RefreshMenu: func(dir string) {
			w.refreshed = append(w.refreshed, dir)
		},
	})
}

// publish adds a Release; see testkit.Releases.Publish.
func (w *world) publish(tag string, published time.Time, assets map[string]testkit.AppImage) map[string][]byte {
	w.t.Helper()
	return w.source.Publish(tag, published, assets)
}

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

type notifierFunc func(core.Notification)

func (f notifierFunc) Notify(_ context.Context, n core.Notification) { f(n) }

// start makes appID's AppImage run, as a fake /proc entry, and returns a
// function that stops it.
func (w *world) start(appID string) (stop func()) {
	w.t.Helper()
	dir := w.path(fmt.Sprintf("proc/%d", 1000+len(appID)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.Symlink(w.path("Applications/"+appID+"/"+appID+".AppImage"), filepath.Join(dir, "exe")); err != nil {
		w.t.Fatal(err)
	}
	return func() { os.RemoveAll(dir) }
}

// tamper changes the bytes served for a published Asset.
func (w *world) tamper(tag, name string) { w.source.Tamper(tag, name) }

// startReplaced makes a process run appID's AppImage as it was before a
// Swap: Linux shows such an exe as "<path> (deleted)". It returns a
// function that stops the process.
func (w *world) startReplaced(appID string) (stop func()) {
	w.t.Helper()
	dir := w.path(fmt.Sprintf("proc/%d", 2000+len(appID)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.Symlink(w.path("Applications/"+appID+"/"+appID+".AppImage")+" (deleted)", filepath.Join(dir, "exe")); err != nil {
		w.t.Fatal(err)
	}
	return func() { os.RemoveAll(dir) }
}
