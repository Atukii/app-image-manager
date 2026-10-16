package core

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"appimage-manager/internal/desktop"
	"appimage-manager/internal/state"
)

// Unmanage ends AiM's responsibility for a Managed App, leaving it
// installed, integrated and working (UNM-1…7).
func (c *Core) Unmanage(ctx context.Context, id string, options ...Opt) (Outcome, error) {
	o := collect(options)
	st, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	app, ok := st.Apps[id]
	if !ok {
		return Outcome{}, fmt.Errorf("%q is not a Managed App", id)
	}
	plan, err := c.unmanagePlan(ctx, o, app, "")
	if err != nil {
		return Outcome{}, err
	}
	return c.runConfirmed(ctx, o, fmt.Sprintf("Stop managing %s? It stays installed and keeps working.", id), plan, st.Generation, func(s *state.State) {
		delete(s.Apps, id)
	})
}

// unmanagePlan asks Unmanage's questions about app and returns its Plan.
// suffix is added to the Question IDs, so Uninstall can ask per app.
func (c *Core) unmanagePlan(ctx context.Context, o opts, app state.App, suffix string) (Plan, error) {
	id := app.ID
	entry := filepath.Join(c.deps.Paths.Applications, app.DesktopFile)
	plan := Plan{
		{Describe: fmt.Sprintf("stop and delete aim-check@%s.timer", id), do: func() error { return c.scheduler.Remove(ctx, id) }},
	}
	previous := c.appImagePath(id) + ".previous"
	if app.Previous != nil {
		answer, err := o.ask(ctx, Question{
			ID:      "unmanage.previous" + suffix,
			Prompt:  fmt.Sprintf("%s keeps %s as its Previous Version. Delete it?", id, app.Previous.Tag),
			Options: []Option{{Key: "delete", Label: "delete " + previous}, {Key: "keep", Label: "keep it"}},
			Default: ptr("delete"), YesOK: true, Flag: "--previous",
		})
		if err != nil {
			return nil, err
		}
		if answer == "delete" {
			plan = append(plan, Step{Describe: "delete Previous Version " + previous, do: func() error { return removeIfExists(previous) }})
		}
	}
	plan = append(plan, Step{Describe: "remove AiM's X-AiM-* markers from " + entry, do: func() error { return stripMarkers(entry) }})
	layout, err := o.ask(ctx, Question{
		ID:     "unmanage.layout" + suffix,
		Prompt: fmt.Sprintf("Leave %s's files in %s, or move them to the standard layout?", id, c.appFolder(id)),
		Options: []Option{
			{Key: "keep", Label: "leave them where they are"},
			{Key: "standard", Label: "move the AppImage to " + c.deps.Paths.Apps + " and the icon to the icon theme"},
		},
		Default: ptr("keep"), YesOK: true, Flag: "--layout",
	})
	if err != nil {
		return nil, err
	}
	if layout == "standard" {
		plan = append(plan, c.standardLayout(app, entry))
	}
	return plan, nil
}

// standardLayout is the Step that moves an unmanaged app out of its App
// Folder: the AppImage to ~/Applications/<Name>.AppImage and the icon into
// the hicolor icon theme, rewriting the Desktop Entry to match (UNM-6).
func (c *Core) standardLayout(app state.App, entry string) Step {
	oldAppImage := c.appImagePath(app.ID)
	newAppImage := filepath.Join(c.deps.Paths.Apps, fileName(c.displayName(app))+".AppImage")
	oldIcon := filepath.Join(c.appFolder(app.ID), app.ID+app.IconExt)
	newIcon := filepath.Join(c.deps.Paths.Icons, "hicolor", iconSize(oldIcon), "apps", app.ID+app.IconExt)
	return Step{
		Describe: fmt.Sprintf("move the AppImage to %s and the icon to %s, and point %s at them", newAppImage, newIcon, entry),
		do: func() error {
			if err := os.MkdirAll(filepath.Dir(newIcon), 0o755); err != nil {
				return err
			}
			if err := moveFile(oldIcon, newIcon); err != nil {
				return err
			}
			if err := moveFile(oldAppImage, newAppImage); err != nil {
				return err
			}
			if _, err := os.Stat(oldAppImage + ".previous"); err == nil {
				if err := moveFile(oldAppImage+".previous", newAppImage+".previous"); err != nil {
					return err
				}
			}
			data, err := os.ReadFile(entry)
			if err != nil {
				return err
			}
			rewritten := strings.NewReplacer(oldAppImage, newAppImage, oldIcon, newIcon).Replace(string(data))
			if err := os.WriteFile(entry, []byte(rewritten), 0o644); err != nil {
				return err
			}
			os.Remove(c.appFolder(app.ID)) // only succeeds once empty
			return nil
		},
	}
}

// fileName makes name safe as a file name without quoting: letters,
// digits, dots, underscores and dashes.
func fileName(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, name)
}

// iconSize is the hicolor size directory for an icon file: "scalable" for
// SVG, the PNG's dimensions, or 256x256 when they can't be read.
func iconSize(path string) string {
	if strings.HasSuffix(path, ".svg") {
		return "scalable"
	}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if cfg, err := png.DecodeConfig(f); err == nil {
			return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)
		}
	}
	return "256x256"
}

// stripMarkers removes the X-AiM-* keys from the Desktop Entry at path.
func stripMarkers(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entry, err := desktop.Parse(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, desktop.StripAiM(entry).Render(), 0o644)
}
