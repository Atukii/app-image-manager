package core

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"appimage-manager/internal/appimage"
	"appimage-manager/internal/asset"
	"appimage-manager/internal/desktop"
	"appimage-manager/internal/source"
	"appimage-manager/internal/state"
)

//go:embed default-icon.svg
var defaultIcon []byte

// ErrNoAsset means a Release has no AppImage AiM could install.
var ErrNoAsset = errors.New("no x86_64 AppImage in the release")

// Outcome is what an operation did, or with DryRun would do.
type Outcome struct {
	Plan []string `json:"plan"`
	// App is the Managed App the operation was about.
	App string `json:"app,omitempty"`
}

// Add installs the latest Release found at sourceURL as a new Managed App.
func (c *Core) Add(ctx context.Context, sourceURL string, options ...Opt) (Outcome, error) {
	o := collect(options)
	ref, err := source.Resolve(sourceURL)
	if err != nil {
		return Outcome{}, err
	}
	src, err := c.deps.Sources(ref)
	if err != nil {
		return Outcome{}, err
	}
	rel, err := src.Latest(ctx)
	if err != nil {
		return Outcome{}, fmt.Errorf("finding the latest release: %w", err)
	}
	chosen, pattern, err := chooseAsset(ctx, o, ref, rel)
	if err != nil {
		return Outcome{}, err
	}
	download, err := c.fetch(ctx, o, chosen)
	if err != nil {
		return Outcome{}, err
	}
	info, err := appimage.Inspect(download)
	if err != nil {
		return Outcome{}, err
	}
	current, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	id, err := askAppID(ctx, o, suggestID(ref, chosen.Name), current)
	if err != nil {
		return Outcome{}, err
	}

	app := state.App{
		ID: id, Source: ref, AssetPattern: pattern,
		Tag: rel.Tag, Published: rel.Published, Poll: c.deps.DefaultPoll,
		DesktopFile: info.DesktopFile,
	}
	if app.DesktopFile == "" {
		app.DesktopFile = id + ".desktop"
	}
	plan := c.installPlan(ctx, &app, download, info)
	if o.dryRun {
		return Outcome{Plan: plan.Descriptions(), App: id}, nil
	}
	_, err = c.store.Update(func(s *state.State) error {
		if _, taken := s.Apps[id]; taken {
			return fmt.Errorf("app ID %q: %w", id, ErrAppIDTaken)
		}
		if err := plan.run(); err != nil {
			return err
		}
		s.Apps[id] = app
		return nil
	})
	return Outcome{Plan: plan.Descriptions(), App: id}, err
}

// chooseAsset picks the Asset to install, asking when several match, and
// returns it with the Asset Pattern to use for future Releases.
func chooseAsset(ctx context.Context, o opts, ref source.Ref, rel source.Release) (source.Asset, string, error) {
	pattern := ""
	if ref.Asset != "" {
		pattern = asset.Pattern(ref.Asset, ref.Tag)
	}
	names := make([]string, len(rel.Assets))
	for i, a := range rel.Assets {
		names[i] = a.Name
	}
	name, candidates := asset.Pick(names, pattern)
	switch {
	case len(candidates) == 0:
		return source.Asset{}, "", fmt.Errorf("%s has %q: %w", rel.Tag, names, ErrNoAsset)
	case name == "":
		q := Question{ID: "asset", Prompt: fmt.Sprintf("%s has several AppImages. Which one should be installed?", rel.Tag), Flag: "--asset"}
		for i, c := range candidates {
			q.Options = append(q.Options, Option{Key: fmt.Sprint(i + 1), Label: c})
		}
		answer, err := o.ask(ctx, q)
		if err != nil {
			return source.Asset{}, "", err
		}
		for _, opt := range q.Options {
			if answer == opt.Key || answer == opt.Label {
				name = opt.Label
			}
		}
		if name == "" {
			q.Problem = fmt.Sprintf("%q is not one of the options", answer)
			q.Basis = basis(q)
			return source.Asset{}, "", &NeedAnswers{Questions: []Question{q}}
		}
	}
	if pattern == "" {
		pattern = asset.Pattern(name, rel.Tag)
	}
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, pattern, nil
		}
	}
	panic("picked asset not in release")
}

// fetch downloads a verified Asset into the download cache, or reuses one
// already there (ADR 0008: replays don't download again).
func (c *Core) fetch(ctx context.Context, o opts, a source.Asset) (string, error) {
	key := strings.TrimPrefix(a.Digest, "sha256:")
	if key == "" {
		key = slug(a.URL)
	}
	dest := filepath.Join(c.deps.Paths.Cache, "downloads", key, a.Name)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := source.Download(ctx, c.deps.HTTP, a, dest, o.progress); err != nil {
		return "", err
	}
	return dest, nil
}

// ErrAppIDTaken means another Managed App already has the App ID.
var ErrAppIDTaken = errors.New("already in use")

// askAppID asks for the new app's permanent App ID.
func askAppID(ctx context.Context, o opts, suggested string, current state.State) (string, error) {
	q := Question{
		ID: "app-id",
		Prompt: "App ID (names its folder and files; it can't be changed later " +
			"without removing and re-adding the app)",
		Default: ptr(suggested), YesOK: true, Flag: "--app-id",
	}
	if _, taken := current.Apps[suggested]; taken {
		q.Default, q.Problem = nil, fmt.Sprintf("%q is already in use", suggested)
	}
	id, err := o.ask(ctx, q)
	if err != nil {
		return "", err
	}
	problem := ""
	if _, taken := current.Apps[id]; taken {
		problem = fmt.Sprintf("%q is already in use", id)
	} else if id == "" || slug(id) != id {
		problem = fmt.Sprintf("%q must be lowercase letters, digits and dashes", id)
	}
	if problem != "" {
		q.Problem = problem
		q.Basis = basis(q)
		return "", &NeedAnswers{Questions: []Question{q}}
	}
	return id, nil
}

// suggestID proposes an App ID: the repository name (or, for a Direct URL,
// the file name), without suffixes like "-releases".
func suggestID(ref source.Ref, assetName string) string {
	name := ref.Repo
	if ref.Kind == source.Direct {
		name = strings.TrimSuffix(strings.TrimSuffix(assetName, filepath.Ext(assetName)), ".AppImage")
	}
	id := slug(name)
	for _, suffix := range []string{"-releases", "-appimage"} {
		if trimmed := strings.TrimSuffix(id, suffix); trimmed != "" {
			id = trimmed
		}
	}
	return id
}

// installPlan is the Plan that puts a downloaded AppImage in place as app:
// App Folder, AppImage, icon, Desktop Entry and timer.
func (c *Core) installPlan(ctx context.Context, app *state.App, download string, info appimage.Info) Plan {
	folder, appImage := c.appFolder(app.ID), c.appImagePath(app.ID)
	icon, ext := info.Icon, info.IconExt
	if len(icon) == 0 {
		icon, ext = defaultIcon, ".svg"
	}
	app.IconExt = ext
	iconPath := filepath.Join(folder, app.ID+ext)
	entryPath := filepath.Join(c.deps.Paths.Applications, app.DesktopFile)

	return Plan{
		{
			Describe: "create " + folder,
			do:       func() error { return os.MkdirAll(folder, 0o755) },
			undo:     func() { os.RemoveAll(folder) },
		},
		{
			Describe: "install " + appImage,
			do: func() error {
				if err := moveFile(download, appImage); err != nil {
					return err
				}
				return os.Chmod(appImage, 0o755)
			},
		},
		{
			Describe: "write icon " + iconPath,
			do:       func() error { return os.WriteFile(iconPath, icon, 0o644) },
		},
		{
			Describe: "write Desktop Entry " + entryPath,
			do: func() error {
				entry, err := c.desktopEntry(*app, info.DesktopEntry, iconPath)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(c.deps.Paths.Applications, 0o755); err != nil {
					return err
				}
				return os.WriteFile(entryPath, entry, 0o644)
			},
			undo: func() { os.Remove(entryPath) },
		},
		{
			Describe: fmt.Sprintf("check for updates %s (aim-check@%s.timer)", app.Poll, app.ID),
			do:       func() error { return c.scheduler.Set(ctx, app.ID, app.Poll) },
			undo:     func() { c.scheduler.Remove(ctx, app.ID) },
		},
	}
}

// desktopEntry builds app's Desktop Entry from its Embedded Desktop Entry
// (or a Draft when it has none) and its Overrides.
func (c *Core) desktopEntry(app state.App, embedded []byte, iconPath string) ([]byte, error) {
	entry := desktop.Draft(app.ID)
	if len(embedded) > 0 {
		var err error
		if entry, err = desktop.Parse(embedded); err != nil {
			return nil, err
		}
	}
	target := desktop.Target{AppID: app.ID, AppImage: c.appImagePath(app.ID), Icon: iconPath}
	return desktop.Build(entry, target, app.Overrides).Render(), nil
}

// moveFile renames src to dst, copying when they are on different
// filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return err
	}
	return os.Remove(src)
}
