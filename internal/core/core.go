// Package core performs every AiM operation. Callers (the aim CLI, and
// through it aim-gui) answer its Questions with an Answerer (ADR 0008).
package core

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"appimage-manager/internal/schedule"
	"appimage-manager/internal/source"
	"appimage-manager/internal/state"
)

// Paths are the directories AiM works in.
type Paths struct {
	Apps         string // ~/Applications: one App Folder per Managed App
	Data         string // ~/.local/share/aim: state and staging
	Applications string // ~/.local/share/applications: Desktop Entries
	Units        string // ~/.config/systemd/user
	Cache        string // ~/.cache/aim: downloads and API responses
	Icons        string // ~/.local/share/icons: the user's icon themes
	Config       string // ~/.config/aim
	Autostart    string // ~/.config/autostart: where the tray's login entry lives
	// ScanDirs are where Scan looks for AppImages AiM doesn't manage yet.
	ScanDirs []string
	// SystemApplications are the system Desktop Entry directories, checked
	// for Shadowing.
	SystemApplications []string
}

// HomePaths are the standard Paths for a home directory.
func HomePaths(home string) Paths {
	return Paths{
		Apps:               filepath.Join(home, "Applications"),
		Data:               filepath.Join(home, ".local/share/aim"),
		Applications:       filepath.Join(home, ".local/share/applications"),
		Units:              filepath.Join(home, ".config/systemd/user"),
		Cache:              filepath.Join(home, ".cache/aim"),
		Icons:              filepath.Join(home, ".local/share/icons"),
		Config:             filepath.Join(home, ".config/aim"),
		Autostart:          filepath.Join(home, ".config/autostart"),
		SystemApplications: []string{"/usr/local/share/applications", "/usr/share/applications"},
		ScanDirs: []string{
			filepath.Join(home, "Applications"), filepath.Join(home, "Downloads"),
			filepath.Join(home, ".local/bin"), filepath.Join(home, "AppImages"),
		},
	}
}

// Deps is everything Core needs from the outside world.
type Deps struct {
	Paths    Paths
	HTTP     *http.Client
	Now      func() time.Time
	ProcRoot string // normally /proc
	// Sources opens the Release Source a Ref points at.
	Sources   func(source.Ref) (source.ReleaseSource, error)
	Systemctl schedule.Systemctl
	// Aim is the path of the aim binary that timers run.
	Aim string
	// DefaultPoll is the Poll Rate for new apps; weekly when empty.
	DefaultPoll state.PollRate
	// Sleep waits between Check retries; time.Sleep when nil.
	Sleep func(time.Duration)
	// Notifier tells the user about Checks; nil sends nothing.
	Notifier Notifier
	// Binaries are the installed aim and aim-gui executables.
	Binaries []string
	// PackageOwner names the package that installed path (pacman -Qo), or
	// returns "" when no package did. Nil means none did.
	PackageOwner func(path string) string
	// HasFuse2 reports whether libfuse2, which older AppImages need, is
	// installed. Nil checks the usual library paths.
	HasFuse2 func() bool
	// RefreshMenu tells the desktop that the Desktop Entries in dir changed
	// (update-desktop-database). Nil does nothing.
	RefreshMenu func(dir string)
}

// Core performs AiM operations.
type Core struct {
	deps      Deps
	store     *state.Store
	scheduler *schedule.Scheduler
}

// New returns a Core.
func New(d Deps) *Core {
	if d.HTTP == nil {
		d.HTTP = http.DefaultClient
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.ProcRoot == "" {
		d.ProcRoot = "/proc"
	}
	if d.HasFuse2 == nil {
		d.HasFuse2 = func() bool {
			for _, lib := range []string{"/usr/lib/libfuse.so.2", "/usr/lib64/libfuse.so.2", "/usr/lib/x86_64-linux-gnu/libfuse.so.2"} {
				if _, err := os.Stat(lib); err == nil {
					return true
				}
			}
			return false
		}
	}
	if d.PackageOwner == nil {
		d.PackageOwner = func(string) string { return "" }
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if d.Notifier == nil {
		d.Notifier = silent{}
	}
	if d.RefreshMenu == nil {
		d.RefreshMenu = func(string) {}
	}
	if d.DefaultPoll == "" {
		d.DefaultPoll = state.Weekly
	}
	return &Core{
		deps:      d,
		store:     state.Open(d.Paths.Data),
		scheduler: &schedule.Scheduler{UnitDir: d.Paths.Units, Aim: d.Aim, Systemctl: d.Systemctl},
	}
}

func (c *Core) appFolder(id string) string { return filepath.Join(c.deps.Paths.Apps, id) }
func (c *Core) appImagePath(id string) string {
	return filepath.Join(c.appFolder(id), id+".AppImage")
}

// Notification is a message for the user about a Managed App (SWAP-4).
type Notification struct {
	Kind    string `json:"kind"` // installed | staged | failed | digest-mismatch
	App     string `json:"app"`
	Summary string `json:"summary"`
}

// Notifier shows Notifications, e.g. over D-Bus.
type Notifier interface {
	Notify(ctx context.Context, n Notification)
}

type silent struct{}

func (silent) Notify(context.Context, Notification) {}
