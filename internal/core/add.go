package core

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
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
	// Notes are one-line messages for the user.
	Notes []string `json:"notes,omitempty"`
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
	if o.icon != "" {
		if info.Icon, info.IconExt, err = readIcon(o.icon); err != nil {
			return Outcome{}, err
		}
	}
	poll := c.deps.DefaultPoll
	if o.poll != "" {
		if !validPoll(o.poll) {
			return Outcome{}, fmt.Errorf("poll rate %q: want daily, weekly, monthly or never", o.poll)
		}
		poll = o.poll
	}
	current, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	facts := downloadFacts(chosen, rel.Tag)
	id, err := askAppID(ctx, o, suggestID(ref, chosen.Name), current, facts)
	if err != nil {
		return Outcome{}, err
	}

	app := state.App{
		ID: id, Source: ref, AssetPattern: pattern,
		Tag: rel.Tag, Published: rel.Published, Poll: poll,
		DesktopFile: info.DesktopFile, UserIcon: o.icon != "",
	}
	if app.DesktopFile == "" {
		app.DesktopFile = id + ".desktop"
	}
	if app.DesktopFile, err = c.avoidShadowing(ctx, o, id, app.DesktopFile); err != nil {
		return Outcome{}, err
	}
	if len(info.DesktopEntry) == 0 {
		if app.Overrides, err = reviewDraft(ctx, o, id); err != nil {
			return Outcome{}, err
		}
	}
	plan := c.installPlan(ctx, &app, download, info)
	if o.dryRun {
		return Outcome{Plan: plan.Descriptions(), App: id}, nil
	}
	out := Outcome{Plan: plan.Descriptions(), App: id}
	firstTimer := !c.scheduler.Installed() && app.Poll != state.Never
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
	if err != nil {
		return out, err
	}
	c.deps.RefreshMenu(c.deps.Paths.Applications)
	out.Notes = append(out.Notes, verifiedNote(facts))
	if firstTimer {
		out.Notes = append(out.Notes, c.timersNote())
	}
	return out, nil
}

// downloadFacts describes a downloaded Asset for the user (ADD-5).
func downloadFacts(a source.Asset, tag string) map[string]string {
	verified := "no"
	if a.Digest != "" {
		verified = "yes" // fetch only keeps downloads that match their Digest
	}
	return map[string]string{
		"file": a.Name, "version": tag, "size": fmt.Sprintf("%.1f MB", float64(a.Size)/1e6), "verified": verified,
	}
}

// verifiedNote tells the user whether the download was verified.
func verifiedNote(facts map[string]string) string {
	if facts["verified"] == "yes" {
		return "Verified " + facts["file"] + " against the checksum its release publishes."
	}
	return "The release publishes no checksum for " + facts["file"] + ", so AiM couldn't verify the download."
}

// timersNote tells the user the first Managed App installed AiM's timers
// (ADD-11).
func (c *Core) timersNote() string {
	return "Installed AiM's systemd user timers in " + c.deps.Paths.Units + " so updates are checked automatically."
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

// askAppID asks for the new app's permanent App ID, showing facts about
// what is being installed.
func askAppID(ctx context.Context, o opts, suggested string, current state.State, facts map[string]string) (string, error) {
	q := Question{
		Facts: facts,
		ID:    "app-id",
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

// readIcon reads a user-supplied icon, which must be a PNG or SVG file.
func readIcon(path string) ([]byte, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".svg" {
		return nil, "", fmt.Errorf("icon %s: must be a .png or .svg file", path)
	}
	data, err := os.ReadFile(path)
	return data, ext, err
}

// avoidShadowing checks whether a Desktop Entry named file would hide a
// system one (ADD-9) and, if so, asks whether to use <id>.desktop instead.
func (c *Core) avoidShadowing(ctx context.Context, o opts, id, file string) (string, error) {
	for _, dir := range c.deps.Paths.SystemApplications {
		system := filepath.Join(dir, file)
		if _, err := os.Stat(system); err != nil {
			continue
		}
		renamed := id + ".desktop"
		answer, err := o.ask(ctx, Question{
			ID: "shadowing",
			Prompt: fmt.Sprintf("%s already exists. A user Desktop Entry with the same name would hide it "+
				"from the application menu.", system),
			Options: []Option{
				{Key: "rename", Label: "use " + renamed + " instead (both stay visible)"},
				{Key: "hide", Label: "use " + file + " anyway (hides the system entry)"},
			},
			Default: ptr("rename"), YesOK: true, Flag: "--shadowing",
		})
		if err != nil {
			return "", err
		}
		if answer == "hide" {
			return file, nil
		}
		return renamed, nil
	}
	return file, nil
}

// draftFields are the drafted Desktop Entry values the user reviews
// (ADD-8a), in the order they are asked.
var draftFields = []struct{ key, prompt string }{
	{"Name", "Name shown in the application menu"},
	{"Comment", "Short description (optional)"},
	{"Categories", "Categories, ;-separated, from: AudioVideo Development Education Game " +
		"Graphics Network Office Science Settings System Utility"},
	{"Keywords", "Search keywords, ;-separated (optional)"},
	{"Terminal", "Run in a terminal? (true or false)"},
}

// reviewDraft asks the user to review the Desktop Entry AiM drafted for an
// AppImage without one. Values the user changes become Overrides.
func reviewDraft(ctx context.Context, o opts, id string) (desktop.Overrides, error) {
	draft := desktop.Draft(id)
	overrides := desktop.Overrides{Fields: map[string]string{}}
	for _, f := range draftFields {
		suggested, _ := draft.Get(f.key)
		value, err := o.ask(ctx, Question{
			ID:      "entry." + f.key,
			Prompt:  "This AppImage has no Desktop Entry, so AiM drafted one. " + f.prompt,
			Default: ptr(suggested), YesOK: true, Flag: "--entry " + f.key + "=…",
		})
		if err != nil {
			return desktop.Overrides{}, err
		}
		if value != suggested {
			overrides.Fields[f.key] = value
		}
	}
	return overrides, nil
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
	entry, entryErr := c.desktopEntry(*app, info.DesktopEntry, iconPath)

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
			// Put the file back before the App Folder is removed: when
			// adopting, it is the user's own AppImage.
			undo: func() { moveFile(appImage, download) },
		},
		{
			Describe: "write icon " + iconPath,
			do:       func() error { return os.WriteFile(iconPath, icon, 0o644) },
		},
		{
			// The preview is exactly what is written (ADOPT-3).
			Describe: "write Desktop Entry " + entryPath + ":\n" + string(entry),
			do: func() error {
				if entryErr != nil {
					return entryErr
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
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile copies src to dst and flushes it to disk.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
