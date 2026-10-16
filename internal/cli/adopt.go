package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"appimage-manager/internal/core"
)

func init() {
	commands["adopt"] = command{
		usage: "adopt <appimage> <source-url> [--version V | --install-latest] [--keep Key=a|b|c]… " +
			"[--delete-old-entries yes|no] [add flags] [--yes] [--dry-run]",
		minArgs: 2, maxArgs: 2,
		flags: adoptFlags,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return inv.env.Core.Adopt(ctx, args[0], args[1], inv.options()...)
		},
		print: printOutcome("Adopted"),
	}
	commands["ignore"] = command{
		usage: "ignore <appimage>", minArgs: 1, maxArgs: 1,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return nil, inv.env.Core.Ignore(ctx, args[0])
		},
		print: func(inv *invocation, _ any) { fmt.Fprintln(inv.env.Stdout, "AiM will leave it alone.") },
	}
	commands["scan"] = command{
		usage: "scan [adopt flags]",
		flags: adoptFlags,
		run:   scan,
	}
}

func adoptFlags(fs *flag.FlagSet, inv *invocation) {
	newAppFlags(fs, inv)
	inv.answer(fs, "version", "adopt.version", "the adopted AppImage's version")
	fs.BoolFunc("install-latest", "replace the adopted AppImage with the latest release", func(string) error {
		inv.fixed["adopt.version"] = core.Answer{Value: "latest"}
		return nil
	})
	fs.Func("keep", "for a Desktop Entry field, keep a) your value, b) the AppImage's or c) both: Key=a|b|c (repeatable)", func(v string) error {
		key, choice, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("want Key=a|b|c, got %q", v)
		}
		inv.fixed["consolidate."+key] = core.Answer{Value: choice}
		return nil
	})
	inv.answer(fs, "delete-old-entries", "adopt.delete-old", "yes or no: delete your old Desktop Entries for the AppImage")
}

// scan offers each AppImage AiM doesn't manage yet: manage it, leave it
// alone for good, or skip it until the next scan (SCAN-2).
func scan(ctx context.Context, inv *invocation, _ []string) (any, error) {
	found, err := inv.env.Core.Scan(ctx)
	if err != nil || inv.json {
		return found, err
	}
	if len(found) == 0 {
		fmt.Fprintln(inv.env.Stdout, "No AppImages found that AiM doesn't manage.")
		return nil, nil
	}
	if !inv.env.Interactive {
		for _, f := range found {
			fmt.Fprintln(inv.env.Stdout, f.Path)
		}
		fmt.Fprintln(inv.env.Stdout, "Run aim scan in a terminal to choose what to do with each, or aim adopt <appimage> <source-url>.")
		return nil, nil
	}
	ask := inv.prompter()
	for _, f := range found {
		choice, ok := ask.Answer(ctx, core.Question{
			Prompt: "Found " + f.Path,
			Options: []core.Option{
				{Key: "m", Label: "manage it with AiM"},
				{Key: "l", Label: "leave it alone (don't ask again)"},
				{Key: "s", Label: "skip it for now"},
			},
			Default: ptrTo("s"),
		})
		if !ok {
			return nil, nil
		}
		switch choice.Value {
		case "l":
			if err := inv.env.Core.Ignore(ctx, f.Path); err != nil {
				return nil, err
			}
		case "m":
			q := core.Question{Prompt: "Its GitHub repository or download link:"}
			if f.SourceURL != "" {
				q.Default = &f.SourceURL
			}
			url, ok := ask.Answer(ctx, q)
			if !ok {
				return nil, nil
			}
			out, err := inv.env.Core.Adopt(ctx, f.Path, url.Value, inv.options()...)
			if err != nil {
				fmt.Fprintf(inv.env.Stderr, "aim: %v\n", err)
				continue
			}
			printOutcome("Adopted")(inv, out)
		}
	}
	return nil, nil
}

func ptrTo(s string) *string { return &s }
