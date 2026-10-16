package core

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appimage-manager/internal/source"
	"appimage-manager/internal/state"
)

// Problem is something wrong that doctor found, with how to fix it (DOC-1).
type Problem struct {
	App     string `json:"app,omitempty"`
	Problem string `json:"problem"`
	Fix     string `json:"fix"`
}

// Doctor checks AiM's setup and every Managed App for problems.
func (c *Core) Doctor(ctx context.Context) ([]Problem, error) {
	var found []Problem
	if !c.deps.HasFuse2() {
		found = append(found, Problem{
			Problem: "fuse2 is not installed; many AppImages need it to start",
			Fix:     "sudo pacman -S fuse2",
		})
	}
	st, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(st.Apps)) {
		app := st.Apps[id]
		if app.Poll != state.Never && !c.scheduler.Enabled(ctx, id) {
			found = append(found, Problem{App: id,
				Problem: fmt.Sprintf("its %s update checks are not scheduled (aim-check@%s.timer is missing or disabled)", app.Poll, id),
				Fix:     "aim setup"})
		}
		found = append(found, c.fileProblems(app)...)
	}
	for _, id := range c.scheduler.Scheduled() {
		if _, managed := st.Apps[id]; !managed {
			found = append(found, Problem{
				Problem: fmt.Sprintf("aim-check@%s.timer is left over for %s, which AiM no longer manages", id, id),
				Fix:     "aim setup"})
		}
	}
	if len(st.Apps) == 0 && len(c.scheduler.Scheduled()) == 0 && c.scheduler.Installed() {
		found = append(found, Problem{
			Problem: "AiM's timer templates are left over in " + c.deps.Paths.Units + " with no apps to check",
			Fix:     "aim setup"})
	}
	for _, app := range st.Apps {
		if app.LastCheck != nil && strings.Contains(app.LastCheck.Error, source.ErrRateLimited.Error()) {
			found = append(found, Problem{
				Problem: "GitHub's limit of 60 requests an hour without a token stopped update checks",
				Fix:     "set github_token in " + filepath.Join(c.deps.Paths.Config, "config.toml") + " (or the GITHUB_TOKEN environment variable)"})
			break
		}
	}
	return found, nil
}

// fileProblems checks a Managed App's App Folder, AppImage and Desktop
// Entry.
func (c *Core) fileProblems(app state.App) []Problem {
	id := app.ID
	if _, err := os.Stat(c.appFolder(id)); err != nil {
		return []Problem{{App: id,
			Problem: "its App Folder " + c.appFolder(id) + " is gone",
			Fix:     "aim remove " + id + " (then add it again if you still want it)"}}
	}
	var found []Problem
	appImage := c.appImagePath(id)
	if info, err := os.Stat(appImage); err != nil {
		found = append(found, Problem{App: id, Problem: appImage + " is missing",
			Fix: "aim remove " + id + ", then add it again"})
	} else if info.Mode().Perm()&0o111 == 0 {
		found = append(found, Problem{App: id, Problem: appImage + " is not executable",
			Fix: "chmod +x " + appImage})
	}
	entry := filepath.Join(c.deps.Paths.Applications, app.DesktopFile)
	if _, err := os.Stat(entry); err != nil {
		found = append(found, Problem{App: id, Problem: "its Desktop Entry " + entry + " is missing, so it isn't in the application menu",
			Fix: "aim setup"})
	}
	for _, dir := range c.deps.Paths.SystemApplications {
		if system := filepath.Join(dir, app.DesktopFile); fileExists(system) {
			found = append(found, Problem{App: id,
				Problem: entry + " hides " + system + " (installed by a system package) from the application menu",
				Fix:     "aim remove " + id + " and add it again choosing " + id + ".desktop, or uninstall the system package"})
		}
	}
	return found
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
