package core_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"appimage-manager/internal/core"
	"appimage-manager/internal/schedule"
	"appimage-manager/internal/source"
)

func problems(t *testing.T, w *world) []core.Problem {
	t.Helper()
	found, err := w.core().Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// onlyProblem fails unless doctor reports exactly one problem whose fix
// mentions fix.
func onlyProblem(t *testing.T, w *world, fix string) core.Problem {
	t.Helper()
	found := problems(t, w)
	if len(found) != 1 || !strings.Contains(found[0].Fix, fix) {
		t.Fatalf("got %+v; want one problem fixed with %q", found, fix)
	}
	return found[0]
}

func TestDoctorFindsNothingWrongWithAHealthyInstall(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	if found := problems(t, w); len(found) != 0 {
		t.Fatalf("got %+v; want no problems", found)
	}
}

func TestDoctorReportsMissingFuse2(t *testing.T) {
	w := newWorld(t)
	w.noFuse2 = true

	onlyProblem(t, w, "sudo pacman -S fuse2")
}

func TestDoctorReportsATimerThatWasTurnedOff(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.systemd.Run(context.Background(), "--user", "disable", "--now", "aim-check@obsidian.timer")

	if p := onlyProblem(t, w, "aim setup"); p.App != "obsidian" {
		t.Fatalf("got %+v; want it reported for obsidian", p)
	}
}

func TestDoctorReportsARecordWithoutAnAppFolder(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	os.RemoveAll(w.path("Applications/obsidian"))

	onlyProblem(t, w, "aim remove obsidian")
}

func TestDoctorReportsAnAppImageThatIsNotExecutable(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	os.Chmod(w.path("Applications/obsidian/obsidian.AppImage"), 0o644)

	onlyProblem(t, w, "chmod +x "+w.path("Applications/obsidian/obsidian.AppImage"))
}

func TestDoctorReportsAMissingDesktopEntry(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	os.Remove(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))

	onlyProblem(t, w, "aim setup")
}

func TestDoctorReportsAShadowedSystemEntry(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	systemEntry(t, w, "md.obsidian.Obsidian.desktop") // a package installed later

	p := onlyProblem(t, w, "aim remove obsidian")
	if !strings.Contains(p.Problem, "hides") {
		t.Fatalf("got %+v; want the hidden system entry explained", p)
	}
}

func TestDoctorSuggestsATokenWhenGitHubsRateLimitStoppedChecks(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.source.failWith = fmt.Errorf("GitHub https://api.github.com/repos/obsidianmd/obsidian-releases/releases/latest: %w", source.ErrRateLimited)
	w.core().Check(context.Background(), "obsidian")

	onlyProblem(t, w, "github_token")
}

func TestDoctorReportsTimersLeftForAppsAiMNoLongerManages(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	leftover := &schedule.Scheduler{UnitDir: w.path(".config/systemd/user"), Aim: "aim", Systemctl: w.systemd}
	leftover.Set(context.Background(), "old-app", "weekly")

	if p := onlyProblem(t, w, "aim setup"); !strings.Contains(p.Problem, "old-app") {
		t.Fatalf("got %+v; want the leftover old-app timer named", p)
	}
}

func setup(t *testing.T, w *world) {
	t.Helper()
	if _, err := w.core().Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSetupReEnablesATimerThatWasTurnedOff(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	w.systemd.Run(context.Background(), "--user", "disable", "--now", "aim-check@obsidian.timer")

	setup(t, w)

	if found := problems(t, w); len(found) != 0 {
		t.Fatalf("got %+v; want no problems after setup", found)
	}
}

func TestSetupRewritesAMissingDesktopEntry(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	os.Remove(w.path(".local/share/applications/md.obsidian.Obsidian.desktop"))

	setup(t, w)

	if !strings.Contains(desktopEntry(t, w), "X-AiM-AppId=obsidian") {
		t.Fatal("want the Desktop Entry rebuilt")
	}
}

func TestSetupRemovesTimersLeftForAppsAiMNoLongerManages(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	leftover := &schedule.Scheduler{UnitDir: w.path(".config/systemd/user"), Aim: "aim", Systemctl: w.systemd}
	leftover.Set(context.Background(), "old-app", "weekly")

	setup(t, w)

	if found := problems(t, w); len(found) != 0 || w.systemd.Enabled["aim-check@old-app.timer"] {
		t.Fatalf("got %+v, timers %v; want old-app's timer gone and no problems", found, w.systemd.Enabled)
	}
}

func TestSetupIsSafeToRunAgainAndAgain(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)

	setup(t, w)
	setup(t, w)

	if found := problems(t, w); len(found) != 0 || !w.systemd.Enabled["aim-check@obsidian.timer"] {
		t.Fatalf("got %+v; want everything still healthy", found)
	}
}
