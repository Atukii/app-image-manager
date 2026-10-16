package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appimage-manager/internal/core"
)

// adoptionWithConsolidation prepares an adoption that asks every kind of
// Question: an uncertain version, the App ID, Consolidation choices, the
// old entry and the final confirmation.
func adoptionWithConsolidation(t *testing.T) (*fixture, []string) {
	t.Helper()
	f := newFixture(t)
	path := f.downloaded(t)
	entry := filepath.Join(f.home, ".local/share/applications/my-obsidian.desktop")
	os.MkdirAll(filepath.Dir(entry), 0o755)
	os.WriteFile(entry, []byte("[Desktop Entry]\nType=Application\nName=My Obsidian\nExec="+path+" %U\nCategories=Utility;\n"), 0o644)
	return f, []string{"adopt", path, obsidianURL}
}

var scripted = map[string]string{
	"adopt.version":          "v1.8.2",
	"app-id":                 "obsidian",
	"consolidate.Name":       "a",
	"consolidate.Categories": "c",
	"adopt.delete-old":       "yes",
	"confirm":                "yes",
}

func TestTheTerminalAndTheGUIAskTheSameQuestionsInTheSameOrder(t *testing.T) {
	// The GUI's way: one aim --json call per Question.
	f, args := adoptionWithConsolidation(t)
	answers := core.Fixed{}
	var prompts, order []string
	for round := 0; ; round++ {
		input, _ := json.Marshal(answers)
		code, stdout, _ := f.runWithStdin(string(input), append(args, "--json", "--answers", "-")...)
		if code == 0 {
			break
		}
		var asked struct{ Questions []core.Question }
		if err := json.Unmarshal([]byte(lastLine(stdout)), &asked); err != nil || code != 3 || round > 10 {
			t.Fatalf("round %d: exit %d, stdout %q", round, code, stdout)
		}
		q := asked.Questions[0]
		value, ok := scripted[q.ID]
		if !ok {
			t.Fatalf("unexpected Question %q", q.ID)
		}
		answers[q.ID] = core.Answer{Value: value, Basis: q.Basis}
		prompts = append(prompts, strings.ReplaceAll(q.Prompt, f.home, "~"))
		order = append(order, q.ID)
	}

	// The terminal's way: the same answers, typed in that order.
	g, args := adoptionWithConsolidation(t)
	var typed strings.Builder
	for _, id := range order {
		typed.WriteString(scripted[id] + "\n")
	}
	code, _, stderr := g.run(typed.String(), args...)
	if code != 0 {
		t.Fatalf("terminal: exit %d, stderr:\n%s", code, stderr)
	}
	stderr = strings.ReplaceAll(stderr, g.home, "~")
	at := 0
	for _, prompt := range prompts {
		i := strings.Index(stderr[at:], prompt)
		if i < 0 {
			t.Fatalf("terminal didn't ask %q next; it asked:\n%s", prompt, stderr[at:])
		}
		at += i + len(prompt)
	}
}
