package core_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"appimage-manager/internal/core"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRemoveShowsItsRemovalPlanAndChangesNothingUntilConfirmed(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	_, err := w.core().Remove(context.Background(), "obsidian")

	q := needAnswers(t, err)
	plan := strings.Join(q.Plan, "\n")
	if q.ID != "confirm" || !strings.Contains(plan, w.path("Applications/obsidian")) ||
		!strings.Contains(plan, "md.obsidian.Obsidian.desktop") || !exists(w.path("Applications/obsidian/obsidian.AppImage")) {
		t.Fatalf("got %+v; want a confirm Question listing the App Folder and Desktop Entry, and nothing removed", q)
	}
}

func TestAConfirmedRemoveDeletesTheAppEntirely(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()

	if _, err := w.core().Remove(ctx, "obsidian", core.Yes); err != nil {
		t.Fatal(err)
	}

	apps, _ := w.core().List(ctx)
	if len(apps) != 0 || exists(w.path("Applications/obsidian")) ||
		exists(w.path(".local/share/applications/md.obsidian.Obsidian.desktop")) || len(w.systemd.Enabled) != 0 {
		t.Fatalf("apps %v, timers %v; want obsidian gone entirely", apps, w.systemd.Enabled)
	}
}

func TestAConfirmationOfAnOutdatedPlanIsAskedAgain(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	ctx := context.Background()
	_, err := w.core().Remove(ctx, "obsidian")
	shown := needAnswers(t, err)
	w.core().SetPoll(ctx, "obsidian", "daily") // state changes before the user confirms

	_, err = w.core().Remove(ctx, "obsidian", core.With(core.Fixed{"confirm": {Value: "yes", Basis: shown.Basis}}))

	if q := needAnswers(t, err); q.ID != "confirm" || !exists(w.path("Applications/obsidian")) {
		t.Fatalf("got %+v; want the confirm Question again and nothing removed", q)
	}
}
