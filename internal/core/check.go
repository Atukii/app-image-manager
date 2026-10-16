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
	// Reopen is set when the app was running while it was updated: it
	// uses the new version once reopened.
	Reopen bool `json:"reopen,omitempty"`
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
	a, err := releaseAsset(rel, app.AssetPattern)
	if err != nil {
		return fail(err)
	}
	download, err := c.fetch(ctx, opts{}, a)
	if err != nil {
		return fail(err)
	}
	// The app may be running: the Swap is safe anyway (ADR 0009), and the
	// user is told to reopen it.
	running := proc.Running(c.appImagePath(app.ID), c.deps.ProcRoot)
	if err := c.swap(app.ID, download, state.Release{Tag: rel.Tag, Published: rel.Published}); err != nil {
		return fail(err)
	}
	summary := fmt.Sprintf("%s updated to %s", app.ID, rel.Tag)
	if running {
		summary += "; reopen it to use the new version"
		result.Reopen = true
	}
	c.deps.Notifier.Notify(ctx, Notification{Kind: "installed", App: app.ID, Summary: summary})
	result.Result = "installed"
	return result, nil
}

// swap installs the verified AppImage at download as app id's new
// Version, the Release rel.
func (c *Core) swap(id, download string, rel state.Release) error {
	_, err := c.store.Update(func(s *state.State) error {
		app := s.Apps[id]
		if err := c.swapFiles(&app, download, rel); err != nil {
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

// swapFiles replaces app's installed AppImage with the one at src without
// ever writing into the installed file or leaving its path missing, so a
// running copy keeps working (SWAP-1, ADR 0009): src becomes a temporary
// file in the App Folder, the installed file is hard-linked as the
// Previous Version, and the temporary file is renamed over it. The icon
// and Desktop Entry are then rebuilt; if that fails, the Previous Version
// is renamed back.
func (c *Core) swapFiles(app *state.App, src string, rel state.Release) error {
	current := c.appImagePath(app.ID)
	previous := current + ".previous"
	incoming := filepath.Join(c.appFolder(app.ID), "."+app.ID+".AppImage.new")
	if err := moveFile(src, incoming); err != nil {
		return err
	}
	if err := os.Chmod(incoming, 0o755); err != nil {
		os.Remove(incoming)
		return err
	}
	if err := linkOrCopy(current, previous); err != nil {
		os.Remove(incoming)
		return err
	}
	if err := os.Rename(incoming, current); err != nil {
		os.Remove(incoming)
		return err
	}
	if err := c.reintegrate(app); err != nil {
		os.Rename(previous, current)
		c.reintegrate(app)
		return err
	}
	app.Previous = &state.Release{Tag: app.Tag, Published: app.Published}
	app.Tag, app.Published = rel.Tag, rel.Published
	app.Skipped = "" // a newer Release supersedes the Skipped one
	return nil
}

// linkOrCopy gives the file at path the extra name link, replacing any
// file there: a hard link where the filesystem allows, a copy otherwise.
func linkOrCopy(path, link string) error {
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Link(path, link); err == nil {
		return nil
	}
	return copyFile(path, link)
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
