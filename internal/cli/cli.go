// Package cli is the aim command line: it turns arguments, flags and
// terminal input into core operations and prints their results.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"appimage-manager/internal/core"
	"appimage-manager/internal/state"
)

// Env is the outside world aim runs in.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// Interactive is set when stdin is a terminal, so aim may prompt.
	Interactive bool
	Core        *core.Core
}

// Exit codes.
const (
	exitOK        = 0
	exitError     = 1
	exitUsage     = 2
	exitQuestions = 3 // --json: the operation needs answers
)

// Run runs aim with args (without the program name) and returns its exit
// code.
func Run(ctx context.Context, args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return exitUsage
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(env.Stdout, usage)
		return exitOK
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(env.Stderr, "aim: unknown command %q\n\n%s", args[0], usage)
		return exitUsage
	}
	inv := &invocation{env: env, fixed: core.Fixed{}}
	fs := flag.NewFlagSet("aim "+args[0], flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.BoolVar(&inv.yes, "yes", false, "accept every suggested answer")
	fs.BoolVar(&inv.json, "json", false, "print machine-readable JSON (for aim-gui and scripts)")
	fs.BoolVar(&inv.dryRun, "dry-run", false, "show what would change, and change nothing")
	answersFile := fs.String("answers", "", "read answers from a JSON file, or - for stdin")
	if cmd.flags != nil {
		cmd.flags(fs, inv)
	}
	positional, err := parse(fs, args[1:])
	if err != nil {
		return exitUsage
	}
	if len(positional) < cmd.minArgs || (cmd.maxArgs >= 0 && len(positional) > cmd.maxArgs) {
		fmt.Fprintf(env.Stderr, "usage: aim %s\n", cmd.usage)
		return exitUsage
	}
	if *answersFile != "" {
		if err := inv.readAnswers(*answersFile); err != nil {
			return inv.fail(err)
		}
	}
	result, err := cmd.run(ctx, inv, positional)
	if err != nil {
		return inv.fail(err)
	}
	if inv.json {
		inv.printJSON(map[string]any{"status": "done", "result": result})
	} else if cmd.print != nil {
		cmd.print(inv, result)
	}
	return exitOK
}

// readAnswers adds the answers in a JSON file (or stdin, for "-") mapping
// Question IDs to answers. Answers given as flags take precedence.
func (inv *invocation) readAnswers(path string) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(inv.env.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	var answers core.Fixed
	if err := json.Unmarshal(data, &answers); err != nil {
		return fmt.Errorf("reading answers: %w", err)
	}
	for id, a := range answers {
		if _, fromFlag := inv.fixed[id]; !fromFlag {
			inv.fixed[id] = a
		}
	}
	return nil
}

// printJSON prints v as one line: --json output is a stream of JSON
// objects, progress events first and the result last.
func (inv *invocation) printJSON(v any) {
	json.NewEncoder(inv.env.Stdout).Encode(v)
}

// progress reports download progress: as JSON events with --json,
// otherwise in the terminal. It reports each whole percent once.
func (inv *invocation) progress() func(done, total int64) {
	last := int64(-1)
	return func(done, total int64) {
		step := done >> 20 // per MiB when the size is unknown
		if total > 0 {
			step = done * 100 / total
		}
		if step == last {
			return
		}
		last = step
		switch {
		case inv.json:
			inv.printJSON(map[string]any{"event": "progress", "done": done, "total": total})
		case inv.env.Interactive && total > 0:
			fmt.Fprintf(inv.env.Stderr, "\rDownloading… %d%%", step)
			if done == total {
				fmt.Fprintln(inv.env.Stderr)
			}
		}
	}
}

// parse parses flags wherever they appear among the arguments, returning
// the positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// invocation is one run of an aim command.
type invocation struct {
	env    Env
	yes    bool
	json   bool
	dryRun bool
	fixed  core.Fixed // answers given as flags or with --answers
	edit   core.Edit  // aim edit's changes
	extra  []core.Opt // options from flags such as --poll and --icon
	ask    *prompter  // shared, so buffered terminal input isn't lost
}

// prompter returns the invocation's terminal prompter.
func (inv *invocation) prompter() *prompter {
	if inv.ask == nil {
		inv.ask = newPrompter(inv.env.Stdin, inv.env.Stderr)
	}
	return inv.ask
}

// answer records a flag's value as the answer to Question id.
func (inv *invocation) answer(fs *flag.FlagSet, name, id, help string) {
	fs.Func(name, help, func(v string) error {
		inv.fixed[id] = core.Answer{Value: v}
		return nil
	})
}

// options are the core options for this invocation: flags answer first,
// then the terminal, or --yes's suggested answers.
func (inv *invocation) options() []core.Opt {
	answerers := core.Layered{flagAnswers(inv.fixed)}
	if inv.yes {
		answerers = append(answerers, core.Defaults)
	}
	if inv.env.Interactive {
		answerers = append(answerers, inv.prompter())
	}
	opts := append([]core.Opt{core.With(answerers), core.Progress(inv.progress())}, inv.extra...)
	if inv.dryRun {
		opts = append(opts, core.DryRun())
	}
	return opts
}

// fail reports err and returns the exit code for it.
func (inv *invocation) fail(err error) int {
	if errors.Is(err, errQuietFailure) {
		return exitError
	}
	var need *core.NeedAnswers
	isNeed := errors.As(err, &need)
	if inv.json {
		if isNeed {
			inv.printJSON(map[string]any{"status": "questions", "questions": need.Questions})
			return exitQuestions
		}
		inv.printJSON(map[string]any{"status": "error", "error": err.Error()})
		return exitError
	}
	if isNeed {
		q := need.Questions[0]
		fmt.Fprintf(inv.env.Stderr, "aim: %s\n", q.Prompt)
		if q.Problem != "" {
			fmt.Fprintf(inv.env.Stderr, "aim: %s\n", q.Problem)
		}
		if q.Flag != "" {
			fmt.Fprintf(inv.env.Stderr, "aim: answer with %s\n", q.Flag)
		}
		return exitUsage
	}
	fmt.Fprintf(inv.env.Stderr, "aim: %v\n", err)
	return exitError
}

// errQuietFailure exits 1 without a message: the command already said
// what went wrong.
var errQuietFailure = errors.New("failed")

// flagAnswers answers Questions from flags. A flag answers both the
// Question with its ID and the per-app ones Uninstall asks ("<id>:<app>").
type flagAnswers core.Fixed

func (f flagAnswers) Answer(ctx context.Context, q core.Question) (core.Answer, bool) {
	if a, ok := core.Fixed(f).Answer(ctx, q); ok {
		return a, true
	}
	if base, _, perApp := strings.Cut(q.ID, ":"); perApp {
		q.ID = base
		return core.Fixed(f).Answer(ctx, q)
	}
	return core.Answer{}, false
}

// command is one aim subcommand.
type command struct {
	usage            string
	minArgs, maxArgs int // maxArgs < 0: no limit
	flags            func(*flag.FlagSet, *invocation)
	// run performs the command, returning its result for --json.
	run func(context.Context, *invocation, []string) (any, error)
	// print shows the result to a person; nil prints nothing.
	print func(*invocation, any)
}

var commands = map[string]command{
	"add": {
		usage:   "add <source-url> [--app-id ID] [--asset N] [--poll RATE] [--icon FILE] [--entry Key=Value]… [--shadowing rename|hide] [--yes] [--dry-run]",
		minArgs: 1, maxArgs: 1,
		flags: newAppFlags,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return inv.env.Core.Add(ctx, args[0], inv.options()...)
		},
		print: printOutcome("Added"),
	},
}

// newAppFlags are the flags of commands that create a Managed App.
func newAppFlags(fs *flag.FlagSet, inv *invocation) {
	inv.answer(fs, "app-id", "app-id", "the new app's permanent App ID")
	inv.answer(fs, "asset", "asset", "which AppImage to install when a release has several")
	inv.answer(fs, "shadowing", "shadowing", "rename (use <app-id>.desktop) or hide, when a system entry has the same name")
	fs.Func("poll", "check for updates daily, weekly, monthly or never", func(v string) error {
		inv.extra = append(inv.extra, core.Poll(state.PollRate(v)))
		return nil
	})
	fs.Func("icon", "use this PNG or SVG as the icon", func(v string) error {
		inv.extra = append(inv.extra, core.Icon(v))
		return nil
	})
	fs.Func("entry", "a Desktop Entry value for an AppImage without one, Key=Value (repeatable)", func(v string) error {
		key, value, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("want Key=Value, got %q", v)
		}
		inv.fixed["entry."+key] = core.Answer{Value: value}
		return nil
	})
}

// printOutcome prints an operation's Outcome: with --dry-run its Plan,
// otherwise what was done and any notes.
func printOutcome(done string) func(*invocation, any) {
	return func(inv *invocation, result any) {
		out := result.(core.Outcome)
		w := inv.env.Stdout
		if inv.dryRun {
			fmt.Fprintln(w, "Nothing changed (--dry-run). The plan:")
			for _, step := range out.Plan {
				fmt.Fprintf(w, "  • %s\n", strings.ReplaceAll(step, "\n", "\n    "))
			}
			return
		}
		if out.App != "" {
			fmt.Fprintf(w, "%s %s.\n", done, out.App)
		} else {
			fmt.Fprintf(w, "%s.\n", done)
		}
		for _, note := range out.Notes {
			fmt.Fprintln(w, note)
		}
	}
}

var usage = strings.TrimSpace(`
usage: aim <command> [arguments] [--yes] [--dry-run] [--json]

Getting apps
  add <source-url>             install an AppImage from a GitHub repository or download link
  adopt <appimage> <url>       let AiM manage an AppImage you already have
  scan                         find AppImages AiM doesn't manage yet, and choose what to do with each
  ignore <appimage>            never offer an AppImage in scan again
  inspect <appimage>           show an AppImage's suggested source

Keeping them updated
  list                         show your apps, their versions and update checks
  check [app-id…]              check for and install updates (what the timers run)
  update [app-id…]             the same, by hand
  rollback <app-id>            go back to the previous version, skipping the one rolled back from
  skip <app-id> [version]      don't install a release (the latest by default)
  unskip <app-id>              allow a skipped release again
  set-poll <app-id> <rate>     check daily, weekly, monthly or never

Changing an app
  edit <app-id>                change its name, categories, icon, launch arguments or environment

Letting go
  remove <app-id>              delete an app entirely
  unmanage <app-id>            stop managing an app; it stays installed and keeps working
  uninstall                    stop managing every app and remove AiM itself

Maintenance
  doctor                       look for problems and say how to fix them
  setup                        install or repair the timers and Desktop Entries

Run aim <command> -h for a command's flags.
`) + "\n"
