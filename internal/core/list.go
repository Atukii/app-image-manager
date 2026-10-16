package core

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appimage-manager/internal/desktop"
	"appimage-manager/internal/proc"
	"appimage-manager/internal/source"
	"appimage-manager/internal/state"
)

// AppStatus is how a Managed App stands (LIST-1).
type AppStatus struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Poll    state.PollRate `json:"poll"`
	// LastCheck is nil until the app is first Checked.
	LastCheck *state.LastCheck `json:"lastCheck,omitempty"`
	// Skipped is the Skipped Release's tag.
	Skipped string `json:"skipped,omitempty"`
	// Reopen is set while the app still runs the version an update
	// replaced (SWAP-2).
	Reopen bool `json:"reopen,omitempty"`
	// Previous is the Previous Version's tag, available for Rollback.
	Previous string `json:"previous,omitempty"`
	// Source is the Release Source's URL.
	Source       string `json:"source"`
	AssetPattern string `json:"assetPattern"`
	DesktopFile  string `json:"desktopFile"`
	// Overrides are the user's Desktop Entry values and launch settings.
	Overrides desktop.Overrides `json:"overrides"`
}

// List returns every Managed App, sorted by App ID.
func (c *Core) List(ctx context.Context) ([]AppStatus, error) {
	st, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	var apps []AppStatus
	for _, id := range slices.Sorted(maps.Keys(st.Apps)) {
		app := st.Apps[id]
		status := AppStatus{ID: id, Name: c.displayName(app), Version: app.Tag, Poll: app.Poll, LastCheck: app.LastCheck, Skipped: app.Skipped, Overrides: app.Overrides,
			Reopen: proc.RunningReplaced(c.appImagePath(id), c.deps.ProcRoot),
			Source: sourceURL(app.Source), AssetPattern: app.AssetPattern, DesktopFile: app.DesktopFile}
		if app.Previous != nil {
			status.Previous = app.Previous.Tag
		}
		apps = append(apps, status)
	}
	return apps, nil
}

// displayName is the Name in app's Desktop Entry, or its App ID.
func (c *Core) displayName(app state.App) string {
	data, err := os.ReadFile(filepath.Join(c.deps.Paths.Applications, app.DesktopFile))
	if err != nil {
		return app.ID
	}
	entry, err := desktop.Parse(data)
	if err != nil {
		return app.ID
	}
	if name, ok := entry.Get("Name"); ok && strings.TrimSpace(name) != "" {
		return name
	}
	return app.ID
}

// sourceURL is the URL a Release Source was added from, in its canonical
// form.
func sourceURL(ref source.Ref) string {
	if ref.Kind == source.Direct {
		return ref.URL
	}
	return "https://github.com/" + ref.Owner + "/" + ref.Repo
}
