package schedule_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appimage-manager/internal/schedule"
	"appimage-manager/internal/state"
)

func newScheduler(t *testing.T) (*schedule.Scheduler, *fakeSystemd) {
	t.Helper()
	unitDir := t.TempDir()
	systemd := newFakeSystemd(unitDir)
	return &schedule.Scheduler{UnitDir: unitDir, Aim: "/home/jakub/.local/bin/aim", Systemctl: systemd}, systemd
}

func TestSetSchedulesTheAppsOwnTimer(t *testing.T) {
	s, systemd := newScheduler(t)

	if err := s.Set(context.Background(), "obsidian", state.Weekly); err != nil {
		t.Fatal(err)
	}

	dropIn := readFile(filepath.Join(s.UnitDir, "aim-check@obsidian.timer.d", "poll.conf"))
	if !systemd.enabled["aim-check@obsidian.timer"] || !strings.Contains(dropIn, "OnCalendar=weekly\n") {
		t.Fatalf("enabled: %v, drop-in:\n%s\nwant the obsidian timer enabled with OnCalendar=weekly", systemd.enabled, dropIn)
	}
}

func TestSetNeverRemovesTheAppsTimer(t *testing.T) {
	s, systemd := newScheduler(t)
	ctx := context.Background()
	if err := s.Set(ctx, "obsidian", state.Weekly); err != nil {
		t.Fatal(err)
	}

	if err := s.Set(ctx, "obsidian", state.Never); err != nil {
		t.Fatal(err)
	}

	_, statErr := os.Stat(filepath.Join(s.UnitDir, "aim-check@obsidian.timer.d"))
	if systemd.enabled["aim-check@obsidian.timer"] || !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("enabled: %v, drop-in dir error: %v; want the timer disabled and its drop-in gone", systemd.enabled, statErr)
	}
}

func TestSetNeverWorksForTheVeryFirstApp(t *testing.T) {
	s, _ := newScheduler(t)

	if err := s.Set(context.Background(), "obsidian", state.Never); err != nil {
		t.Fatalf("got %v; want no error when nothing was ever scheduled", err)
	}
}

func templatesExist(s *schedule.Scheduler) bool {
	_, serviceErr := os.Stat(filepath.Join(s.UnitDir, "aim-check@.service"))
	_, timerErr := os.Stat(filepath.Join(s.UnitDir, "aim-check@.timer"))
	return serviceErr == nil && timerErr == nil
}

func TestRemoveKeepsTheTemplatesWhileOtherAppsAreScheduled(t *testing.T) {
	s, systemd := newScheduler(t)
	ctx := context.Background()
	s.Set(ctx, "obsidian", state.Weekly)
	s.Set(ctx, "neovim", state.Daily)

	if err := s.Remove(ctx, "obsidian"); err != nil {
		t.Fatal(err)
	}

	if systemd.enabled["aim-check@obsidian.timer"] || !systemd.enabled["aim-check@neovim.timer"] || !templatesExist(s) {
		t.Fatalf("enabled: %v, templates: %v; want only neovim's timer, and the templates kept", systemd.enabled, templatesExist(s))
	}
}

func TestRemovingTheLastAppLeavesNoAiMUnits(t *testing.T) {
	s, systemd := newScheduler(t)
	ctx := context.Background()
	s.Set(ctx, "obsidian", state.Weekly)

	if err := s.Remove(ctx, "obsidian"); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(s.UnitDir)
	if len(systemd.enabled) != 0 || len(entries) != 0 {
		t.Fatalf("enabled: %v, unit dir: %v; want nothing left", systemd.enabled, entries)
	}
}
