package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"appimage-manager/internal/version"
)

// Adopt turns an AppImage the user already has, at path, into a Managed
// App whose Releases come from sourceURL (ADOPT-1…5).
func (c *Core) Adopt(ctx context.Context, path, sourceURL string, options ...Opt) (Outcome, error) {
	o := collect(options)
	ref, err := source.Resolve(sourceURL)
	if err != nil {
		return Outcome{}, err
	}
	src, err := c.deps.Sources(ref)
	if err != nil {
		return Outcome{}, err
	}
	info, err := appimage.Inspect(path)
	if err != nil {
		return Outcome{}, err
	}
	rel, assetName, found, err := identify(ctx, src, path)
	if err != nil {
		return Outcome{}, err
	}
	install := path // the file that becomes the Managed App's AppImage
	if !found {
		latest, err := src.Latest(ctx)
		if err != nil {
			return Outcome{}, fmt.Errorf("finding the latest release: %w", err)
		}
		answer, err := askVersion(ctx, o, path, info, latest.Tag)
		if err != nil {
			return Outcome{}, err
		}
		if answer == "latest" {
			chosen, _, err := chooseAsset(ctx, o, ref, latest)
			if err != nil {
				return Outcome{}, err
			}
			if install, err = c.fetch(ctx, o, chosen); err != nil {
				return Outcome{}, err
			}
			if info, err = appimage.Inspect(install); err != nil {
				return Outcome{}, err
			}
			rel, assetName = latest, chosen.Name
		} else {
			rel, assetName = source.Release{Tag: answer}, filepath.Base(path)
		}
	}
	st, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	id, err := askAppID(ctx, o, suggestID(ref, assetName), st)
	if err != nil {
		return Outcome{}, err
	}
	app := state.App{
		ID: id, Source: ref, AssetPattern: asset.Pattern(assetName, rel.Tag),
		Tag: rel.Tag, Published: rel.Published, Poll: c.deps.DefaultPoll,
		DesktopFile: info.DesktopFile,
	}
	if app.DesktopFile == "" {
		app.DesktopFile = id + ".desktop"
	}
	userEntries, err := c.entriesLaunching(path)
	if err != nil {
		return Outcome{}, err
	}
	switch {
	case len(userEntries) > 0:
		if app.Overrides, err = consolidate(ctx, o, userEntries[0], info, id); err != nil {
			return Outcome{}, err
		}
	case len(info.DesktopEntry) == 0:
		if app.Overrides, err = reviewDraft(ctx, o, id); err != nil {
			return Outcome{}, err
		}
	}
	if app.DesktopFile, err = c.avoidShadowing(ctx, o, id, app.DesktopFile); err != nil {
		return Outcome{}, err
	}
	if o.icon != "" {
		if info.Icon, info.IconExt, err = readIcon(o.icon); err != nil {
			return Outcome{}, err
		}
		app.UserIcon = true
	}
	plan := c.installPlan(ctx, &app, install, info)
	if len(userEntries) > 0 {
		answer, err := o.ask(ctx, Question{
			ID:      "adopt.delete-old",
			Prompt:  fmt.Sprintf("Delete your old Desktop Entries for it (%s), so it isn't listed twice?", strings.Join(userEntries, ", ")),
			Options: []Option{{Key: "yes", Label: "delete them"}, {Key: "no", Label: "keep them"}},
			Default: ptr("yes"), YesOK: true, Flag: "--delete-old-entries",
		})
		if err != nil {
			return Outcome{}, err
		}
		if answer == "yes" {
			for _, file := range userEntries {
				if file == filepath.Join(c.deps.Paths.Applications, app.DesktopFile) {
					continue // AiM's own entry replaces it
				}
				plan = append(plan, Step{Describe: "delete your old Desktop Entry " + file, do: func() error { return removeIfExists(file) }})
			}
		}
	}
	if install != path {
		plan = append(plan, Step{Describe: "delete the replaced " + path, do: func() error { return removeIfExists(path) }})
	}
	firstTimer := !c.scheduler.Installed() && app.Poll != state.Never
	out, err := c.runConfirmed(ctx, o, fmt.Sprintf("Adopt %s as %s?", path, id), plan, st.Generation, func(s *state.State) {
		s.Apps[id] = app
	})
	if err == nil && firstTimer && !o.dryRun {
		out.Notes = append(out.Notes, c.timersNote())
	}
	return out, err
}

// identify finds the recent Release with an Asset whose Digest matches the
// file at path (ADOPT-1a), returning it and the Asset's name.
func identify(ctx context.Context, src source.ReleaseSource, path string) (rel source.Release, assetName string, found bool, err error) {
	digest, err := fileDigest(path)
	if err != nil {
		return source.Release{}, "", false, err
	}
	releases, err := src.Recent(ctx)
	if err != nil {
		return source.Release{}, "", false, err
	}
	for _, rel := range releases {
		for _, a := range rel.Assets {
			if a.Digest == digest {
				return rel, a.Name, true, nil
			}
		}
	}
	return source.Release{}, "", false, nil
}

// askVersion asks the user which Version an AppImage AiM couldn't identify
// is, showing AiM's best guess and where it came from, but never choosing
// for them (ADOPT-1b).
// The answer is a Version, or "latest" to install latestTag instead.
func askVersion(ctx context.Context, o opts, path string, info appimage.Info, latestTag string) (string, error) {
	name := filepath.Base(path)
	guess, from := "", ""
	if entry, err := desktop.Parse(info.DesktopEntry); err == nil && len(info.DesktopEntry) > 0 {
		if v, ok := entry.Get("X-AppImage-Version"); ok && v != "" {
			guess, from = v, "the AppImage's own metadata"
		}
	}
	if guess == "" {
		if v, ok := version.FromFilename(name); ok {
			guess, from = v, "its filename"
		}
	}
	q := Question{
		ID:   "adopt.version",
		Flag: "--version <v> or --install-latest",
		Prompt: fmt.Sprintf("AiM can't tell for sure which version %s is. "+
			"Type its version, or choose an option.", name),
	}
	if guess != "" {
		q.Prompt = fmt.Sprintf("AiM can't tell for sure which version %s is; its best guess is %s, from %s. "+
			"Type its version, or choose an option.", name, guess, from)
		q.Options = append(q.Options, Option{Key: "guess", Label: "use " + guess})
	}
	q.Options = append(q.Options, Option{Key: "latest", Label: "replace it with the latest release, " + latestTag + ", now"})
	answer, err := o.ask(ctx, q)
	if err != nil {
		return "", err
	}
	if answer == "guess" {
		answer = guess
	}
	return answer, nil
}

// fileDigest is the "sha256:<hex>" Digest of the file at path.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// entriesLaunching finds the user's Desktop Entries whose Exec or TryExec
// runs the AppImage at path (ADOPT-2).
func (c *Core) entriesLaunching(path string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(c.deps.Paths.Applications, "*.desktop"))
	if err != nil {
		return nil, err
	}
	var found []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		entry, err := desktop.Parse(data)
		if err != nil {
			continue
		}
		for _, key := range []string{"Exec", "TryExec"} {
			if value, _ := entry.Get(key); launches(value, path) {
				found = append(found, file)
				break
			}
		}
	}
	return found, nil
}

// launches reports whether an Exec value runs path, as a whole word.
func launches(exec, path string) bool {
	for i := strings.Index(exec, path); i >= 0; {
		end := i + len(path)
		before, after := i == 0 || strings.ContainsRune(" \t\"'", rune(exec[i-1])), end == len(exec) || strings.ContainsRune(" \t\"'", rune(exec[end]))
		if before && after {
			return true
		}
		next := strings.Index(exec[end:], path)
		if next < 0 {
			break
		}
		i = end + next
	}
	return false
}

// consolidate asks, for each value the user's Desktop Entry at userFile
// sets differently from the AppImage's, which to keep, showing both
// (ADOPT-3). The user's own values become Overrides, as do the extra
// arguments and environment in their Exec.
func consolidate(ctx context.Context, o opts, userFile string, info appimage.Info, id string) (desktop.Overrides, error) {
	data, err := os.ReadFile(userFile)
	if err != nil {
		return desktop.Overrides{}, err
	}
	user, err := desktop.Parse(data)
	if err != nil {
		return desktop.Overrides{}, err
	}
	embedded := desktop.Draft(id)
	if len(info.DesktopEntry) > 0 {
		if embedded, err = desktop.Parse(info.DesktopEntry); err != nil {
			return desktop.Overrides{}, err
		}
	}
	result := desktop.Consolidate(user, embedded)
	overrides := result.Carried
	overrides.Fields = map[string]string{}
	for _, choice := range result.Choices {
		q := Question{
			ID: "consolidate." + choice.Key,
			Prompt: fmt.Sprintf("Your Desktop Entry %s and the AppImage disagree on %s. Which should AiM use?",
				filepath.Base(userFile), choice.Key),
			Options: []Option{{Key: "a", Label: quoted(choice.Yours)}, {Key: "b", Label: quoted(choice.FromAppImage)}},
			Default: ptr("a"), YesOK: true, Flag: "--keep " + choice.Key + "=a|b|c",
		}
		if choice.Both != "" {
			q.Options = append(q.Options, Option{Key: "c", Label: "both: " + quoted(choice.Both)})
		}
		answer, err := o.ask(ctx, q)
		if err != nil {
			return desktop.Overrides{}, err
		}
		switch answer {
		case "a":
			overrides.Fields[choice.Key] = choice.Yours
		case "c":
			overrides.Fields[choice.Key] = choice.Both
		}
	}
	return overrides, nil
}

func quoted(value string) string {
	if value == "" {
		return "(not set)"
	}
	return `"` + value + `"`
}
