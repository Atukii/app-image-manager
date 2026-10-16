package core

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"appimage-manager/internal/state"
)

// Setup installs or repairs every Managed App's timer and Desktop Entry,
// and removes timers left over for apps AiM no longer manages. It is safe
// to run any number of times (SETUP-1).
func (c *Core) Setup(ctx context.Context) (Outcome, error) {
	var out Outcome
	_, err := c.store.Update(func(s *state.State) error {
		for _, id := range slices.Sorted(maps.Keys(s.Apps)) {
			app := s.Apps[id]
			if err := c.scheduler.Set(ctx, id, app.Poll); err != nil {
				return fmt.Errorf("%s: %w", id, err)
			}
			if !fileExists(filepath.Join(c.deps.Paths.Applications, app.DesktopFile)) && fileExists(c.appImagePath(id)) {
				if err := c.reintegrate(&app); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				out.Plan = append(out.Plan, "rewrote "+id+"'s Desktop Entry")
				s.Apps[id] = app
			}
		}
		for _, id := range c.scheduler.Scheduled() {
			if _, managed := s.Apps[id]; !managed {
				if err := c.scheduler.Remove(ctx, id); err != nil {
					return err
				}
				out.Plan = append(out.Plan, "removed the leftover aim-check@"+id+".timer")
			}
		}
		return c.scheduler.Tidy(ctx)
	})
	if err == nil {
		c.deps.RefreshMenu(c.deps.Paths.Applications)
	}
	return out, err
}
