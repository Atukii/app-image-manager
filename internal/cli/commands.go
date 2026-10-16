package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"appimage-manager/internal/core"
	"appimage-manager/internal/state"
)

func init() {
	commands["list"] = command{
		usage: "list [--json]", maxArgs: 0,
		run: func(ctx context.Context, inv *invocation, _ []string) (any, error) {
			return inv.env.Core.List(ctx)
		},
		print: printList,
	}
	check := command{
		usage: "check [app-id…]", maxArgs: -1,
		run: func(ctx context.Context, inv *invocation, ids []string) (any, error) {
			results, err := inv.env.Core.Check(ctx, ids...)
			if err != nil {
				return nil, err
			}
			for _, r := range results {
				if r.Result == "failed" && !inv.json {
					// Exit non-zero so systemd records the failed run.
					printChecks(inv, results)
					return nil, errQuietFailure
				}
			}
			return results, nil
		},
		print: printChecks,
	}
	commands["check"] = check
	update := check
	update.usage = "update [app-id…]"
	commands["update"] = update
	commands["set-poll"] = command{
		usage: "set-poll <app-id> daily|weekly|monthly|never", minArgs: 2, maxArgs: 2,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return nil, inv.env.Core.SetPoll(ctx, args[0], state.PollRate(args[1]))
		},
		print: func(inv *invocation, _ any) { fmt.Fprintln(inv.env.Stdout, "Done.") },
	}
	commands["remove"] = command{
		usage: "remove <app-id> [--yes] [--dry-run]", minArgs: 1, maxArgs: 1,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return inv.env.Core.Remove(ctx, args[0], inv.options()...)
		},
		print: printOutcome("Removed"),
	}
}

func init() {
	byID := func(usage, done string, op func(*core.Core, context.Context, string) (core.Outcome, error)) command {
		return command{
			usage: usage, minArgs: 1, maxArgs: 1,
			run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
				return op(inv.env.Core, ctx, args[0])
			},
			print: printOutcome(done),
		}
	}
	commands["rollback"] = byID("rollback <app-id>", "Rolled back", (*core.Core).Rollback)
	commands["skip"] = command{
		usage: "skip <app-id> [version]", minArgs: 1, maxArgs: 2,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			version := ""
			if len(args) == 2 {
				version = args[1]
			}
			tag, err := inv.env.Core.Skip(ctx, args[0], version)
			return map[string]string{"app": args[0], "skipped": tag}, err
		},
		print: func(inv *invocation, result any) {
			r := result.(map[string]string)
			fmt.Fprintf(inv.env.Stdout, "%s %s will be skipped (aim unskip %s to allow it).\n", r["app"], r["skipped"], r["app"])
		},
	}
	commands["inspect"] = command{
		usage: "inspect <appimage>", minArgs: 1, maxArgs: 1,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return inv.env.Core.Inspect(ctx, args[0])
		},
		print: func(inv *invocation, result any) {
			f := result.(core.Found)
			if f.SourceURL == "" {
				fmt.Fprintf(inv.env.Stdout, "%s is an AppImage without update information; add it with aim adopt %s <source-url>.\n", f.Path, f.Path)
				return
			}
			fmt.Fprintf(inv.env.Stdout, "%s\nsuggested source: %s\n", f.Path, f.SourceURL)
		},
	}
	commands["unskip"] = byID("unskip <app-id>", "Unskipped", func(c *core.Core, ctx context.Context, id string) (core.Outcome, error) {
		return core.Outcome{App: id}, c.Unskip(ctx, id)
	})

	commands["edit"] = command{
		usage:   "edit <app-id> [--set Key=Value]… [--reset Key]… [--arg A]… [--env K=V]… [--clear-args] [--clear-env] [--icon file] [--reset-icon]",
		minArgs: 1, maxArgs: 1,
		flags: func(fs *flag.FlagSet, inv *invocation) {
			inv.edit.Set = map[string]string{}
			fs.Func("set", "override a Desktop Entry field, Key=Value (repeatable)", func(v string) error {
				key, value, ok := strings.Cut(v, "=")
				if !ok {
					return fmt.Errorf("want Key=Value, got %q", v)
				}
				inv.edit.Set[key] = value
				return nil
			})
			fs.Func("reset", "go back to the AppImage's own value for a field (repeatable)", func(v string) error {
				inv.edit.Reset = append(inv.edit.Reset, v)
				return nil
			})
			fs.Func("arg", "an extra launch argument; replaces the current ones (repeatable)", func(v string) error {
				if inv.edit.Args == nil {
					inv.edit.Args = &[]string{}
				}
				*inv.edit.Args = append(*inv.edit.Args, v)
				return nil
			})
			fs.Func("env", "a KEY=VALUE environment variable; replaces the current ones (repeatable)", func(v string) error {
				if !strings.Contains(v, "=") {
					return fmt.Errorf("want KEY=VALUE, got %q", v)
				}
				if inv.edit.Env == nil {
					inv.edit.Env = &[]string{}
				}
				*inv.edit.Env = append(*inv.edit.Env, v)
				return nil
			})
			fs.BoolFunc("clear-args", "remove the extra launch arguments", func(string) error {
				inv.edit.Args = &[]string{}
				return nil
			})
			fs.BoolFunc("clear-env", "remove the extra environment variables", func(string) error {
				inv.edit.Env = &[]string{}
				return nil
			})
			fs.StringVar(&inv.edit.Icon, "icon", "", "use this PNG or SVG as the icon, kept across updates")
			fs.BoolVar(&inv.edit.ResetIcon, "reset-icon", false, "go back to the AppImage's own icon")
		},
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return core.Outcome{App: args[0]}, inv.env.Core.Edit(ctx, args[0], inv.edit)
		},
		print: printOutcome("Updated"),
	}

	commands["unmanage"] = command{
		usage:   "unmanage <app-id> [--previous delete|keep] [--layout keep|standard] [--yes] [--dry-run]",
		minArgs: 1, maxArgs: 1,
		flags: unmanageFlags,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return inv.env.Core.Unmanage(ctx, args[0], inv.options()...)
		},
		print: printOutcome("AiM no longer manages"),
	}

	commands["uninstall"] = command{
		usage: "uninstall --keep-config|--delete-config [--for-all each|once] [unmanage flags] [--yes] [--dry-run]",
		flags: func(fs *flag.FlagSet, inv *invocation) {
			unmanageFlags(fs, inv)
			fs.BoolFunc("keep-config", "keep ~/.config/aim", func(string) error {
				return inv.setOnce("uninstall.config", "keep", "--keep-config and --delete-config")
			})
			fs.BoolFunc("delete-config", "delete ~/.config/aim", func(string) error {
				return inv.setOnce("uninstall.config", "delete", "--keep-config and --delete-config")
			})
			inv.answer(fs, "for-all", "uninstall.scope", "answer the per-app questions one by one (each) or once for all apps (once)")
		},
		run: func(ctx context.Context, inv *invocation, _ []string) (any, error) {
			return inv.env.Core.Uninstall(ctx, inv.options()...)
		},
		print: printOutcome("AiM is uninstalled; your apps stay installed"),
	}

	commands["setup"] = command{
		usage: "setup",
		run: func(ctx context.Context, inv *invocation, _ []string) (any, error) {
			return inv.env.Core.Setup(ctx)
		},
		print: func(inv *invocation, result any) {
			for _, done := range result.(core.Outcome).Plan {
				fmt.Fprintln(inv.env.Stdout, done)
			}
			fmt.Fprintln(inv.env.Stdout, "Timers and Desktop Entries are in place.")
		},
	}

	commands["doctor"] = command{
		usage: "doctor",
		run: func(ctx context.Context, inv *invocation, _ []string) (any, error) {
			problems, err := inv.env.Core.Doctor(ctx)
			if err != nil || inv.json || len(problems) == 0 {
				return problems, err
			}
			printProblems(inv, problems)
			return nil, errQuietFailure
		},
		print: printProblems,
	}
}

func unmanageFlags(fs *flag.FlagSet, inv *invocation) {
	inv.answer(fs, "previous", "unmanage.previous", "delete or keep the Previous Version")
	inv.answer(fs, "layout", "unmanage.layout", "keep the files where they are, or move them to the standard layout")
}

// setOnce answers Question id with value, refusing a second, different
// answer from conflicting flags.
func (inv *invocation) setOnce(id, value, flags string) error {
	if a, ok := inv.fixed[id]; ok && a.Value != value {
		return fmt.Errorf("choose one of %s", flags)
	}
	inv.fixed[id] = core.Answer{Value: value}
	return nil
}

func printProblems(inv *invocation, result any) {
	problems, _ := result.([]core.Problem)
	if len(problems) == 0 {
		fmt.Fprintln(inv.env.Stdout, "No problems found.")
		return
	}
	for _, p := range problems {
		app := ""
		if p.App != "" {
			app = p.App + ": "
		}
		fmt.Fprintf(inv.env.Stdout, "✗ %s%s\n  fix: %s\n", app, p.Problem, p.Fix)
	}
}

func printList(inv *invocation, result any) {
	apps := result.([]core.AppStatus)
	if len(apps) == 0 {
		fmt.Fprintln(inv.env.Stdout, "No apps yet. Add one with: aim add <github-url>")
		return
	}
	tw := tabwriter.NewWriter(inv.env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "APP\tNAME\tVERSION\tCHECKS\tLAST CHECK\tNOTES")
	for _, a := range apps {
		last := "never"
		if a.LastCheck != nil {
			last = a.LastCheck.At.Local().Format(time.DateTime) + " " + a.LastCheck.Result
		}
		notes := ""
		if a.Reopen {
			notes += "Reopen to use " + a.Version + ". "
		}
		if a.Skipped != "" {
			notes += a.Skipped + " skipped (aim unskip " + a.ID + "). "
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Version, a.Poll, last, notes)
	}
	tw.Flush()
}

func printChecks(inv *invocation, result any) {
	for _, r := range result.([]core.CheckResult) {
		w := inv.env.Stdout
		switch r.Result {
		case "installed":
			if r.Reopen {
				fmt.Fprintf(w, "%s: updated to %s; reopen it to use the new version\n", r.App, r.Tag)
			} else {
				fmt.Fprintf(w, "%s: updated to %s\n", r.App, r.Tag)
			}
		case "up-to-date":
			fmt.Fprintf(w, "%s: up to date (%s)\n", r.App, r.Tag)
		case "skipped":
			fmt.Fprintf(w, "%s: %s is skipped (aim unskip %s to allow it)\n", r.App, r.Tag, r.App)
		default:
			fmt.Fprintf(w, "%s: check failed: %s\n", r.App, r.Error)
		}
	}
}
