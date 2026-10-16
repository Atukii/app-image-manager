package core

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appimage-manager/internal/desktop"
	"appimage-manager/internal/state"
)

// AppStatus is how a Managed App stands (LIST-1).
type AppStatus struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Poll    state.PollRate `json:"poll"`
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
		apps = append(apps, AppStatus{ID: id, Name: c.displayName(app), Version: app.Tag, Poll: app.Poll})
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
