package core

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"appimage-manager/internal/state"
)

// Uninstall Unmanages every Managed App and then removes AiM itself
// (UNI-1…4).
func (c *Core) Uninstall(ctx context.Context, options ...Opt) (Outcome, error) {
	o := collect(options)
	st, err := c.store.Load()
	if err != nil {
		return Outcome{}, err
	}
	ids := slices.Sorted(maps.Keys(st.Apps))
	perApp, suffix := o, func(id string) string { return ":" + id }
	if len(ids) > 1 {
		scope, err := o.ask(ctx, Question{
			ID:     "uninstall.scope",
			Prompt: fmt.Sprintf("Answer the questions about your %d apps one by one, or once for all of them?", len(ids)),
			Options: []Option{
				{Key: "each", Label: "one by one"},
				{Key: "once", Label: "once for all apps"},
			},
			Default: ptr("each"), YesOK: true, Flag: "--for-all",
		})
		if err != nil {
			return Outcome{}, err
		}
		if scope == "once" {
			perApp.answerer = &onceForAll{Answerer: o.answerer, answers: map[string]Answer{}}
			suffix = func(string) string { return "" }
		}
	}
	var plan Plan
	for _, id := range ids {
		appPlan, err := c.unmanagePlan(ctx, perApp, st.Apps[id], suffix(id))
		if err != nil {
			return Outcome{}, err
		}
		plan = append(plan, appPlan...)
	}
	keepConfig, err := o.ask(ctx, Question{
		ID:     "uninstall.config",
		Prompt: fmt.Sprintf("Keep AiM's settings in %s, in case you use AiM again?", c.deps.Paths.Config),
		Options: []Option{
			{Key: "keep", Label: "keep " + c.deps.Paths.Config},
			{Key: "delete", Label: "delete " + c.deps.Paths.Config},
		},
		Flag: "--keep-config or --delete-config",
	})
	if err != nil {
		return Outcome{}, err
	}
	autostart := filepath.Join(c.deps.Paths.Autostart, "aim-gui.desktop")
	plan = append(plan, Step{Describe: "delete the tray's autostart entry " + autostart, do: func() error { return removeIfExists(autostart) }})
	if keepConfig == "delete" {
		plan = append(plan, Step{Describe: "delete " + c.deps.Paths.Config, do: func() error { return os.RemoveAll(c.deps.Paths.Config) }})
	}
	var packages, leftover []string
	for _, bin := range c.deps.Binaries {
		if _, err := os.Stat(bin); err != nil {
			continue
		}
		if pkg := c.deps.PackageOwner(bin); pkg != "" {
			if !slices.Contains(packages, pkg) {
				packages = append(packages, pkg)
			}
			continue
		}
		if syscall.Access(filepath.Dir(bin), writeOK) != nil {
			leftover = append(leftover, bin) // e.g. /usr/local/bin: not AiM's to delete
			continue
		}
		plan = append(plan, Step{Describe: "delete " + bin, do: func() error { return removeIfExists(bin) }})
	}
	// AiM's own state goes last, after the lock it lives under is released.
	forget := Plan{
		{Describe: "delete " + c.deps.Paths.Data, do: func() error { return os.RemoveAll(c.deps.Paths.Data) }},
		{Describe: "delete " + c.deps.Paths.Cache, do: func() error { return os.RemoveAll(c.deps.Paths.Cache) }},
	}
	out := Outcome{Plan: append(plan.Descriptions(), forget.Descriptions()...)}
	if len(packages) > 0 {
		out.Notes = append(out.Notes, "Finish with: sudo pacman -R "+strings.Join(packages, " "))
	}
	if len(leftover) > 0 {
		out.Notes = append(out.Notes, "AiM can't delete "+strings.Join(leftover, " and ")+"; remove them yourself.")
	}
	if o.dryRun {
		return out, nil
	}
	if err := o.confirm(ctx, "Uninstall AiM? Your apps stay installed and keep working.", append(plan, forget...), st.Generation); err != nil {
		return out, err
	}
	_, err = c.store.Update(func(s *state.State) error {
		if s.Generation != st.Generation {
			return ErrStale
		}
		if err := plan.run(); err != nil {
			return err
		}
		clear(s.Apps)
		return nil
	})
	if err != nil {
		return out, err
	}
	c.deps.RefreshMenu(c.deps.Paths.Applications)
	return out, forget.run()
}

// onceForAll reuses the first answer to each Question ID for every app.
type onceForAll struct {
	Answerer
	answers map[string]Answer
}

func (a *onceForAll) Answer(ctx context.Context, q Question) (Answer, bool) {
	if ans, ok := a.answers[q.ID]; ok {
		return ans, true
	}
	q.Prompt += " (your answer applies to every app)"
	ans, ok := a.Answerer.Answer(ctx, q)
	if ok {
		a.answers[q.ID] = ans
	}
	return ans, ok
}

// writeOK is access(2)'s "may write" mode.
const writeOK = 0x2
