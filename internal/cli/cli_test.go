package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appimage-manager/internal/cli"
	"appimage-manager/internal/core"
	"appimage-manager/internal/source"
	"appimage-manager/internal/testkit"
)

const obsidianURL = "https://github.com/obsidianmd/obsidian-releases"

var obsidianImage = testkit.AppImage{
	Files: map[string]string{
		"md.obsidian.Obsidian.desktop": "[Desktop Entry]\nName=Obsidian\nExec=AppRun %U\nType=Application\nCategories=Office;\n",
		"AppRun":                       "#!/bin/sh\n",
	},
}

// fixture is a home directory with a fake GitHub serving Obsidian v1.9.0.
type fixture struct {
	home     string
	releases *testkit.Releases
	core     *core.Core
	settings cli.Settings
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{home: t.TempDir(), releases: testkit.NewReleases(t)}
	f.releases.Publish("v1.9.0", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.9.0.AppImage": obsidianImage})
	paths := core.HomePaths(f.home)
	paths.SystemApplications = nil
	f.settings = cli.Settings{
		File:      filepath.Join(paths.Config, "config.toml"),
		Autostart: filepath.Join(paths.Autostart, "aim-gui.desktop"),
		GUI:       "/home/jakub/.local/bin/aim-gui",
	}
	f.core = core.New(core.Deps{
		Paths:     paths,
		HTTP:      f.releases.Server.Client(),
		ProcRoot:  filepath.Join(f.home, "proc"),
		Sources:   func(source.Ref) (source.ReleaseSource, error) { return f.releases, nil },
		Systemctl: testkit.NewSystemd(paths.Units),
		HasFuse2:  func() bool { return true },
	})
	return f
}

// run runs aim with args, without a terminal unless input is given, and
// returns the exit code and output.
func (f *fixture) run(input string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut,
		Interactive: input != "", Core: f.core, Settings: f.settings,
	})
	return code, out.String(), errOut.String()
}

func (f *fixture) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(f.home, rel))
	return err == nil
}

func TestFlagsAnswerQuestions(t *testing.T) {
	f := newFixture(t)

	code, _, stderr := f.run("", "add", obsidianURL, "--app-id", "notes", "--yes")

	if code != 0 || !f.exists("Applications/notes/notes.AppImage") {
		t.Fatalf("exit %d, stderr %q; want Obsidian installed as notes", code, stderr)
	}
}

func TestWithoutATerminalAMissingAnswerNamesItsFlag(t *testing.T) {
	f := newFixture(t)

	code, _, stderr := f.run("", "add", obsidianURL)

	if code != 2 || !strings.Contains(stderr, "--app-id") || f.exists("Applications/obsidian") {
		t.Fatalf("exit %d, stderr %q; want exit 2 naming --app-id and nothing installed", code, stderr)
	}
}

func TestInATerminalAimAsks(t *testing.T) {
	f := newFixture(t)

	code, _, stderr := f.run("notes\n", "add", obsidianURL)

	if code != 0 || !f.exists("Applications/notes/notes.AppImage") || !strings.Contains(stderr, "App ID") {
		t.Fatalf("exit %d, stderr %q; want the App ID prompt answered with notes", code, stderr)
	}
}

func TestTheGUIProtocolAsksThenAppliesAnswers(t *testing.T) {
	f := newFixture(t)

	code, stdout, _ := f.run("", "add", obsidianURL, "--json")
	var asked struct {
		Status    string          `json:"status"`
		Questions []core.Question `json:"questions"`
	}
	if err := json.Unmarshal([]byte(lastLine(stdout)), &asked); err != nil || code != 3 || asked.Status != "questions" || asked.Questions[0].ID != "app-id" {
		t.Fatalf("exit %d, stdout %q; want status questions asking for the app-id", code, stdout)
	}

	answers := fmt.Sprintf(`{"app-id": {"value": "notes", "basis": %q}}`, asked.Questions[0].Basis)
	code, stdout, _ = f.runWithStdin(answers, "add", obsidianURL, "--json", "--answers", "-")
	var done struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(lastLine(stdout)), &done); code != 0 || done.Status != "done" || !f.exists("Applications/notes/notes.AppImage") {
		t.Fatalf("exit %d, stdout %q; want done and notes installed", code, stdout)
	}
}

// runWithStdin runs aim without a terminal, with stdin as its input.
func (f *fixture) runWithStdin(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut, Core: f.core, Settings: f.settings,
	})
	return code, out.String(), errOut.String()
}

func TestJSONReportsDownloadProgressBeforeTheResult(t *testing.T) {
	f := newFixture(t)

	_, stdout, _ := f.run("", "add", obsidianURL, "--yes", "--json")

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var first struct {
		Event       string
		Done, Total int64
	}
	var last struct{ Status string }
	json.Unmarshal([]byte(lines[0]), &first)
	json.Unmarshal([]byte(lines[len(lines)-1]), &last)
	if first.Event != "progress" || first.Done == 0 || last.Status != "done" {
		t.Fatalf("got:\n%s\nwant progress events, one per line, then the done result", stdout)
	}
}

// lastLine is the last line of --json output: the result.
func lastLine(stdout string) string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	return lines[len(lines)-1]
}
