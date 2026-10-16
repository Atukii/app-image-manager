package core

import (
	"context"
	"errors"
	"fmt"
	"os"

	"appimage-manager/internal/state"
)

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
		// Like a Swap, never leave the path missing, so a running app is
		// unaffected (RB-1, ADR 0009): keep a second name for the current
		// file, then rename the Previous Version over the path.
		current := c.appImagePath(id)
		previous, aside := current+".previous", current+".rollback"
		if err := linkOrCopy(current, aside); err != nil {
			return err
		}
		if err := os.Rename(previous, current); err != nil {
			os.Remove(aside)
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

// ErrSkipInstalled means the release to skip is the installed one; going
// back from it is a Rollback.
var ErrSkipInstalled = errors.New("that version is installed; roll back to skip it")

// Skip makes a release of a Managed App, the latest one when tag is empty,
// its Skipped Release, so Checks don't install it (RB-4). It returns the
// skipped tag.
func (c *Core) Skip(ctx context.Context, id, tag string) (string, error) {
	st, err := c.store.Load()
	if err != nil {
		return "", err
	}
	app, ok := st.Apps[id]
	if !ok {
		return "", fmt.Errorf("%q is not a Managed App", id)
	}
	if tag == "" {
		src, err := c.deps.Sources(app.Source)
		if err != nil {
			return "", err
		}
		rel, err := src.Latest(ctx)
		if err != nil {
			return "", fmt.Errorf("finding the latest release: %w", err)
		}
		tag = rel.Tag
	}
	if tag == app.Tag {
		return "", fmt.Errorf("%s %s: %w", id, tag, ErrSkipInstalled)
	}
	_, err = c.store.Update(func(s *state.State) error {
		app := s.Apps[id]
		app.Skipped = tag
		s.Apps[id] = app
		return nil
	})
	return tag, err
}
