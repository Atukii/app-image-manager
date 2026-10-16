// Package core performs every AiM operation. Callers (the aim CLI, and
// through it aim-gui) answer its Questions with an Answerer (ADR 0008).
package core

import (
	"net/http"
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
		SystemApplications: []string{"/usr/local/share/applications", "/usr/share/applications"},
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
