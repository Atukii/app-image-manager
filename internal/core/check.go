package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"appimage-manager/internal/appimage"
	"appimage-manager/internal/asset"
	"appimage-manager/internal/proc"
	"appimage-manager/internal/source"
	"appimage-manager/internal/state"
	"appimage-manager/internal/version"
)

// CheckResult is the outcome of Checking one Managed App.
type CheckResult struct {
	App string `json:"app"`
	// Result is "up-to-date", "installed", "staged" or "failed".
	Result string `json:"result"`
	Tag    string `json:"tag,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Check asks each Managed App's Release Source for a newer Release and
// installs it. Check never asks Questions, so the timer can run it
// unattended. With no ids, every Managed App is Checked.
func (c *Core) Check(ctx context.Context, ids ...string) ([]CheckResult, error) {
	st, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		ids = slices.Sorted(maps.Keys(st.Apps))
	}
	var results []CheckResult
	for _, id := range ids {
		app, ok := st.Apps[id]
		if !ok {
			return results, fmt.Errorf("%q is not a Managed App", id)
		}
		result := c.checkWithRetries(ctx, app)
		c.record(id, result)
		results = append(results, result)
	}
	return results, nil
}

// record keeps result as app id's LastCheck (CHK-7).
func (c *Core) record(id string, result CheckResult) {
	c.store.Update(func(s *state.State) error {
		app, ok := s.Apps[id]
		if !ok {
			return nil
		}
		app.LastCheck = &state.LastCheck{At: c.deps.Now(), Result: result.Result, Error: result.Error}
		s.Apps[id] = app
		return nil
	})
}

// retryWaits are the pauses before each retry of a failed Check (CHK-6).
var retryWaits = []time.Duration{10 * time.Second, 30 * time.Second, 90 * time.Second}

// checkWithRetries Checks app, retrying failures (CHK-6). A Digest
// mismatch is not retried: the Release is rejected and the user told at
// once. Otherwise the user is told only when every attempt failed.
func (c *Core) checkWithRetries(ctx context.Context, app state.App) CheckResult {
	for attempt := 0; ; attempt++ {
		result, err := c.check(ctx, app)
		switch {
		case err == nil, errors.Is(err, errRejected):
			return result
		case errors.Is(err, source.ErrDigestMismatch):
			c.reject(app.ID, result.Tag)
			c.deps.Notifier.Notify(ctx, Notification{Kind: "digest-mismatch", App: app.ID,
				Summary: fmt.Sprintf("%s %s was not installed: the download doesn't match its published checksum", app.ID, result.Tag)})
			return result
		case attempt == len(retryWaits):
			c.deps.Notifier.Notify(ctx, Notification{Kind: "failed", App: app.ID,
				Summary: fmt.Sprintf("Couldn't check %s for updates: %v", app.ID, err)})
			return result
		}
		c.deps.Sleep(retryWaits[attempt])
		if st, err := c.store.Load(); err == nil {
			app = st.Apps[app.ID]
		}
	}
}

// errRejected means the latest Release was rejected by an earlier Check.
var errRejected = errors.New("rejected earlier: its download didn't match the published checksum")

// reject records that tag's download didn't match its Digest.
func (c *Core) reject(id, tag string) {
	c.store.Update(func(s *state.State) error {
		app := s.Apps[id]
		app.Rejected = tag
		s.Apps[id] = app
		return nil
	})
}

// check Checks app once.
func (c *Core) check(ctx context.Context, app state.App) (CheckResult, error) {
	result := CheckResult{App: app.ID}
	fail := func(err error) (CheckResult, error) {
		result.Result, result.Error = "failed", err.Error()
		return result, err
	}
	src, err := c.deps.Sources(app.Source)
	if err != nil {
		return fail(err)
	}
	rel, err := src.Latest(ctx)
	if err != nil {
		return fail(err)
	}
	result.Tag = rel.Tag
	if !version.Newer(version.Release{Tag: rel.Tag, Published: rel.Published},
		version.Release{Tag: app.Tag, Published: app.Published}) {
		result.Result = "up-to-date"
		return result, nil
	}
	if rel.Tag == app.Skipped {
		result.Result = "skipped"
		return result, nil
	}
	if rel.Tag == app.Rejected {
		return fail(fmt.Errorf("%s %w", rel.Tag, errRejected))
	}
	var staged state.Staged
	if app.Staged != nil && !version.Newer(version.Release{Tag: rel.Tag, Published: rel.Published},
		version.Release{Tag: app.Staged.Tag, Published: app.Staged.Published}) {
		// The Staged Update is still the newest Release: use it (SWAP-2).
		staged = *app.Staged
	} else {
		a, err := releaseAsset(rel, app.AssetPattern)
		if err != nil {
			return fail(err)
		}
		download, err := c.fetch(ctx, opts{}, a)
		if err != nil {
			return fail(err)
		}
		if staged, err = c.stage(app.ID, download, rel); err != nil {
			return fail(err)
		}
	}
	if proc.Running(c.appImagePath(app.ID), c.deps.ProcRoot) {
		c.deps.Notifier.Notify(ctx, Notification{Kind: "staged", App: app.ID,
			Summary: fmt.Sprintf("%s %s is ready and will be installed after you close it", app.ID, staged.Tag)})
		result.Result = "staged"
		return result, nil
	}
	if err := c.swap(app.ID, staged); err != nil {
		return fail(err)
	}
	c.deps.Notifier.Notify(ctx, Notification{Kind: "installed", App: app.ID,
		Summary: fmt.Sprintf("%s updated to %s", app.ID, staged.Tag)})
	result.Result = "installed"
	return result, nil
}

// stage moves a verified download into staging as app id's Staged Update,
// replacing any older one.
func (c *Core) stage(id, download string, rel source.Release) (state.Staged, error) {
	dir := filepath.Join(c.deps.Paths.Data, "staging", id)
	staged := state.Staged{Tag: rel.Tag, Published: rel.Published, Path: filepath.Join(dir, filepath.Base(download))}
	_, err := c.store.Update(func(s *state.State) error {
		app := s.Apps[id]
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := moveFile(download, staged.Path); err != nil {
			return err
		}
		app.Staged = &staged
		s.Apps[id] = app
		return nil
	})
	return staged, err
}

// swap replaces app id's installed AppImage with its Staged Update and
// records the new Version.
func (c *Core) swap(id string, staged state.Staged) error {
	_, err := c.store.Update(func(s *state.State) error {
		app := s.Apps[id]
		if err := c.swapFiles(&app, staged); err != nil {
			return err
		}
		s.Apps[id] = app
		return nil
	})
	if err == nil {
		c.deps.RefreshMenu(c.deps.Paths.Applications)
	}
	return err
}

// swapFiles moves staged into place as app's AppImage, keeping the
// installed one as the Previous Version, rebuilds the icon and Desktop
// Entry, and updates app. On failure the installed AppImage is restored.
func (c *Core) swapFiles(app *state.App, staged state.Staged) error {
	current := c.appImagePath(app.ID)
	previous := current + ".previous"
	if err := os.Rename(current, previous); err != nil {
		return err
	}
	if err := moveFile(staged.Path, current); err != nil {
		os.Rename(previous, current)
		return err
	}
	if err := os.Chmod(current, 0o755); err != nil {
		return err
	}
	if err := c.reintegrate(app); err != nil {
		os.Rename(previous, current)
		return err
	}
	app.Previous = &state.Release{Tag: app.Tag, Published: app.Published}
	app.Tag, app.Published, app.Staged = staged.Tag, staged.Published, nil
	app.Skipped = "" // a newer Release supersedes the Skipped one
	return nil
}

// reintegrate rebuilds app's icon (unless the user chose it) and Desktop
// Entry from its installed AppImage and Overrides (SWAP-3, ADR 0004).
func (c *Core) reintegrate(app *state.App) error {
	info, err := appimage.Inspect(c.appImagePath(app.ID))
	if err != nil {
		return err
	}
	if !app.UserIcon {
		icon, ext := info.Icon, info.IconExt
		if len(icon) == 0 {
			icon, ext = defaultIcon, ".svg"
		}
		old := filepath.Join(c.appFolder(app.ID), app.ID+app.IconExt)
		if err := os.WriteFile(filepath.Join(c.appFolder(app.ID), app.ID+ext), icon, 0o644); err != nil {
			return err
		}
		if ext != app.IconExt {
			os.Remove(old)
			app.IconExt = ext
		}
	}
	iconPath := filepath.Join(c.appFolder(app.ID), app.ID+app.IconExt)
	entry, err := c.desktopEntry(*app, info.DesktopEntry, iconPath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.deps.Paths.Applications, app.DesktopFile), entry, 0o644)
}

// releaseAsset is the one Asset of rel matching pattern.
func releaseAsset(rel source.Release, pattern string) (source.Asset, error) {
	names := make([]string, len(rel.Assets))
	for i, a := range rel.Assets {
		names[i] = a.Name
	}
	name, candidates := asset.Pick(names, pattern)
	if name == "" {
		return source.Asset{}, fmt.Errorf("%s: %d assets match %q, want exactly one: %w", rel.Tag, len(candidates), pattern, ErrNoAsset)
	}
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	panic("picked asset not in release")
}
