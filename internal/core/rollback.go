package core

import (
	"context"
	"errors"
	"fmt"
	"os"

	"appimage-manager/internal/proc"
	"appimage-manager/internal/state"
)

// ErrRunning means the Managed App is running, so its AppImage can't be
// swapped (ADR 0005).
var ErrRunning = errors.New("the app is running; close it and try again")

// ErrNoPreviousVersion means there is no Previous Version to roll back to.
var ErrNoPreviousVersion = errors.New("no previous version to roll back to")

// Rollback swaps a Managed App back to its Previous Version (RB-1).
func (c *Core) Rollback(ctx context.Context, id string) (Outcome, error) {
	out := Outcome{App: id}
	_, err := c.store.Update(func(s *state.State) error {
		app, ok := s.Apps[id]
		if !ok {
			return fmt.Errorf("%q is not a Managed App", id)
		}
		if app.Previous == nil {
			return fmt.Errorf("%s: %w", id, ErrNoPreviousVersion)
		}
		current := c.appImagePath(id)
		if proc.Running(current, c.deps.ProcRoot) {
			return fmt.Errorf("%s: %w", id, ErrRunning)
		}
		previous, aside := current+".previous", current+".rollback"
		if err := os.Rename(current, aside); err != nil {
			return err
		}
		if err := os.Rename(previous, current); err != nil {
			os.Rename(aside, current)
			return err
		}
		if err := c.reintegrate(&app); err != nil {
			os.Rename(current, previous)
			os.Rename(aside, current)
			c.reintegrate(&app)
			return err
		}
		os.Remove(aside)
		app.Skipped = app.Tag
		app.Tag, app.Published, app.Previous = app.Previous.Tag, app.Previous.Published, nil
		s.Apps[id] = app
		return nil
	})
	return out, err
}

// Unskip clears a Managed App's Skipped Release, so the next Check may
// install it again (RB-3).
func (c *Core) Unskip(ctx context.Context, id string) error {
	_, err := c.store.Update(func(s *state.State) error {
		app, ok := s.Apps[id]
		if !ok {
			return fmt.Errorf("%q is not a Managed App", id)
		}
		app.Skipped = ""
		s.Apps[id] = app
		return nil
	})
	return err
}
