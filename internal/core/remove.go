package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"appimage-manager/internal/state"
)

// Remove deletes a Managed App entirely: its timer, Desktop Entry, App
// Folder and any Staged Update, after the user confirms the Removal Plan
// (RM-1).
func (c *Core) Remove(ctx context.Context, id string, options ...Opt) (Outcome, error) {
	o := collect(options)
	st, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	app, ok := st.Apps[id]
	if !ok {
		return Outcome{}, fmt.Errorf("%q is not a Managed App", id)
	}
	entry := filepath.Join(c.deps.Paths.Applications, app.DesktopFile)
	staging := filepath.Join(c.deps.Paths.Data, "staging", id)
	plan := Plan{
		{Describe: fmt.Sprintf("stop and delete aim-check@%s.timer", id), do: func() error { return c.scheduler.Remove(ctx, id) }},
		{Describe: "delete Desktop Entry " + entry, do: func() error { return removeIfExists(entry) }},
		{Describe: "delete App Folder " + c.appFolder(id) + " (the AppImage, Previous Version and icon)", do: func() error { return os.RemoveAll(c.appFolder(id)) }},
		{Describe: "delete any Staged Update in " + staging, do: func() error { return os.RemoveAll(staging) }},
	}
	return c.runConfirmed(ctx, o, fmt.Sprintf("Remove %s? This deletes the app.", id), plan, st.Generation, func(s *state.State) {
		delete(s.Apps, id)
	})
}

// runConfirmed asks the user to confirm plan, then runs it under the state
// lock if state is still at generation, and records the change with done.
func (c *Core) runConfirmed(ctx context.Context, o opts, prompt string, plan Plan, generation int, done func(*state.State)) (Outcome, error) {
	out := Outcome{Plan: plan.Descriptions()}
	if o.dryRun {
		return out, nil
	}
	if err := o.confirm(ctx, prompt, plan, generation); err != nil {
		return out, err
	}
	_, err := c.store.Update(func(s *state.State) error {
		if s.Generation != generation {
			return ErrStale
		}
		if err := plan.run(); err != nil {
			return err
		}
		done(s)
		return nil
	})
	if err == nil {
		c.deps.RefreshMenu(c.deps.Paths.Applications)
	}
	return out, err
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
