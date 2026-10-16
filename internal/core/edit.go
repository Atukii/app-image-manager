package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appimage-manager/internal/state"
)

// Edit is a change to a Managed App's Overrides (EDIT-1).
type Edit struct {
	// Set gives Desktop Entry fields the user's value.
	Set map[string]string
	// Reset removes the Overrides of these fields, bringing back the
	// Embedded Desktop Entry's values (EDIT-2).
	Reset []string
	// Args and Env, when not nil, replace the extra launch arguments and
	// KEY=VALUE environment variables; an empty list clears them.
	Args, Env *[]string
	// Icon, when set, is a PNG or SVG file to use as the app's icon from now
	// on, kept across updates.
	Icon string
	// ResetIcon goes back to the AppImage's own icon (or AiM's default).
	ResetIcon bool
}

// EditableFields are the Desktop Entry fields Overrides may set; the rest
// (Exec, TryExec, Icon, X-AiM-*) are AiM's to manage.
var EditableFields = []string{"Name", "GenericName", "Comment", "Categories", "Keywords", "Terminal"}

// Edit changes a Managed App's Overrides and rebuilds its Desktop Entry at
// once (EDIT-3).
func (c *Core) Edit(ctx context.Context, id string, e Edit) error {
	for key := range e.Set {
		if !slices.Contains(EditableFields, key) {
			return fmt.Errorf("%s can't be overridden; editable fields are %s", key, strings.Join(EditableFields, ", "))
		}
	}
	_, err := c.store.Update(func(s *state.State) error {
		app, ok := s.Apps[id]
		if !ok {
			return fmt.Errorf("%q is not a Managed App", id)
		}
		if app.Overrides.Fields == nil {
			app.Overrides.Fields = map[string]string{}
		}
		for key, value := range e.Set {
			app.Overrides.Fields[key] = value
		}
		for _, key := range e.Reset {
			delete(app.Overrides.Fields, key)
		}
		if e.Args != nil {
			app.Overrides.Args = *e.Args
		}
		if e.Env != nil {
			app.Overrides.Env = *e.Env
		}
		if e.ResetIcon {
			app.UserIcon = false
		}
		if e.Icon != "" {
			if err := c.useIcon(&app, e.Icon); err != nil {
				return err
			}
		}
		if err := c.reintegrate(&app); err != nil {
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

// useIcon copies a user-chosen icon into app's App Folder.
func (c *Core) useIcon(app *state.App, path string) error {
	data, ext, err := readIcon(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.appFolder(app.ID), app.ID+ext), data, 0o644); err != nil {
		return err
	}
	if ext != app.IconExt {
		os.Remove(filepath.Join(c.appFolder(app.ID), app.ID+app.IconExt))
	}
	app.IconExt, app.UserIcon = ext, true
	return nil
}

// SetPoll changes how often a Managed App is Checked, rewriting only its
// own timer (EDIT-4).
func (c *Core) SetPoll(ctx context.Context, id string, rate state.PollRate) error {
	if !validPoll(rate) {
		return fmt.Errorf("poll rate %q: want daily, weekly, monthly or never", rate)
	}
	_, err := c.store.Update(func(s *state.State) error {
		app, ok := s.Apps[id]
		if !ok {
			return fmt.Errorf("%q is not a Managed App", id)
		}
		if err := c.scheduler.Set(ctx, id, rate); err != nil {
			return err
		}
		app.Poll = rate
		s.Apps[id] = app
		return nil
	})
	return err
}

func validPoll(rate state.PollRate) bool {
	return slices.Contains([]state.PollRate{state.Daily, state.Weekly, state.Monthly, state.Never}, rate)
}
