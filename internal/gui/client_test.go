package gui_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/gui"
)

// fakeAim replies like aim --json would, and records how it was called.
type fakeAim struct {
	lines []string // printed in order: events, then the reply
	args  []string
	stdin string
}

func (f *fakeAim) Run(_ context.Context, args []string, stdin []byte, line func([]byte)) error {
	f.args, f.stdin = args, string(stdin)
	for _, l := range f.lines {
		line([]byte(l))
	}
	return nil
}

func TestTheClientReportsProgressThenTheResult(t *testing.T) {
	aim := &fakeAim{lines: []string{
		`{"event":"progress","done":50,"total":100}`,
		`{"event":"progress","done":100,"total":100}`,
		`{"status":"done","result":{"app":"obsidian","plan":["…"]}}`,
	}}
	var seen []int64

	reply, err := gui.NewClient(aim).Do(context.Background(), []string{"add", "https://github.com/obsidianmd/obsidian-releases"}, nil,
		func(done, total int64) { seen = append(seen, done) })
	if err != nil {
		t.Fatal(err)
	}

	var out core.Outcome
	json.Unmarshal(reply.Result, &out)
	if reply.Status != "done" || out.App != "obsidian" || !reflect.DeepEqual(seen, []int64{50, 100}) {
		t.Fatalf("got %+v, progress %v", reply, seen)
	}
	if strings.Join(aim.args, " ") != "add https://github.com/obsidianmd/obsidian-releases --json" {
		t.Fatalf("ran aim %q; want the command with --json", aim.args)
	}
}

func TestTheClientSendsAnswersOnStdin(t *testing.T) {
	aim := &fakeAim{lines: []string{`{"status":"questions","questions":[{"id":"consolidate.Name","prompt":"…","basis":"b1"}]}`}}

	reply, err := gui.NewClient(aim).Do(context.Background(), []string{"adopt", "f", "u"},
		core.Fixed{"app-id": {Value: "obsidian", Basis: "b0"}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if reply.Status != "questions" || reply.Questions[0].ID != "consolidate.Name" {
		t.Fatalf("got %+v; want the next Question", reply)
	}
	if !strings.HasSuffix(strings.Join(aim.args, " "), "--json --answers -") || !strings.Contains(aim.stdin, `"app-id"`) {
		t.Fatalf("ran aim %q with stdin %q; want the answers on stdin", aim.args, aim.stdin)
	}
}

func TestTheClientTurnsAnErrorReplyIntoAnError(t *testing.T) {
	aim := &fakeAim{lines: []string{`{"status":"error","error":"\"notes\" is not a Managed App"}`}}

	_, err := gui.NewClient(aim).Do(context.Background(), []string{"rollback", "notes"}, nil, nil)

	if err == nil || !strings.Contains(err.Error(), "is not a Managed App") {
		t.Fatalf("got %v; want aim's message", err)
	}
}

func TestTheClientListsApps(t *testing.T) {
	aim := &fakeAim{lines: []string{`{"status":"done","result":[{"id":"obsidian","name":"Obsidian","version":"v1.10.0","poll":"weekly","reopen":true}]}`}}

	apps, err := gui.NewClient(aim).List(context.Background())

	if err != nil || len(apps) != 1 || apps[0].Name != "Obsidian" || !apps[0].Reopen {
		t.Fatalf("got %+v, %v", apps, err)
	}
}
