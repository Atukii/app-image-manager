// Package schedule runs each Managed App's Checks from its own systemd user
// timer (ADR 0002).
package schedule

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"appimage-manager/internal/state"
)

// Systemctl runs systemctl with args, e.g. ("--user", "daemon-reload").
type Systemctl interface {
	Run(ctx context.Context, args ...string) error
}

// Scheduler manages the aim-check@ timer template and one timer instance per
// Managed App.
type Scheduler struct {
	// UnitDir is the systemd user unit directory, ~/.config/systemd/user.
	UnitDir string
	// Aim is the path of the aim binary the timers run.
	Aim       string
	Systemctl Systemctl
}

const (
	serviceTemplate = "aim-check@.service"
	timerTemplate   = "aim-check@.timer"
)

func timer(appID string) string { return "aim-check@" + appID + ".timer" }

// Installed reports whether the aim-check@ templates are in place.
func (s *Scheduler) Installed() bool {
	_, err := os.Stat(filepath.Join(s.UnitDir, timerTemplate))
	return err == nil
}

// Set schedules appID's Checks at rate. Never means appID has no timer.
func (s *Scheduler) Set(ctx context.Context, appID string, rate state.PollRate) error {
	if rate == state.Never {
		return s.unschedule(ctx, appID)
	}
	if err := s.writeTemplates(); err != nil {
		return err
	}
	dropIn := filepath.Join(s.UnitDir, timer(appID)+".d")
	if err := os.MkdirAll(dropIn, 0o755); err != nil {
		return err
	}
	conf := fmt.Sprintf("[Timer]\nOnCalendar=%s\n", rate)
	if err := os.WriteFile(filepath.Join(dropIn, "poll.conf"), []byte(conf), 0o644); err != nil {
		return err
	}
	if err := s.Systemctl.Run(ctx, "--user", "daemon-reload"); err != nil {
		return err
	}
	return s.Systemctl.Run(ctx, "--user", "enable", "--now", timer(appID))
}

// Remove deletes appID's timer. When no other Managed App has a timer left,
// the aim-check@ templates are deleted too, so no AiM units remain.
func (s *Scheduler) Remove(ctx context.Context, appID string) error {
	if err := s.unschedule(ctx, appID); err != nil {
		return err
	}
	others, err := filepath.Glob(filepath.Join(s.UnitDir, "aim-check@*.timer.d"))
	if err != nil || len(others) > 0 {
		return err
	}
	for _, name := range []string{serviceTemplate, timerTemplate} {
		if err := os.Remove(filepath.Join(s.UnitDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return s.Systemctl.Run(ctx, "--user", "daemon-reload")
}

// unschedule stops and disables appID's timer and deletes its drop-in. An
// app without a drop-in was never scheduled, so there is nothing to do.
func (s *Scheduler) unschedule(ctx context.Context, appID string) error {
	if _, err := os.Stat(filepath.Join(s.UnitDir, timer(appID)+".d")); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := s.Systemctl.Run(ctx, "--user", "disable", "--now", timer(appID)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.UnitDir, timer(appID)+".d")); err != nil {
		return err
	}
	return s.Systemctl.Run(ctx, "--user", "daemon-reload")
}

// writeTemplates writes the aim-check@ service and timer templates (TMR-1,
// TMR-2).
func (s *Scheduler) writeTemplates() error {
	if err := os.MkdirAll(s.UnitDir, 0o755); err != nil {
		return err
	}
	units := map[string]string{
		serviceTemplate: "[Unit]\nDescription=AiM: check %i for updates\n\n" +
			"[Service]\nType=oneshot\nExecStart=" + s.Aim + " check %i\n",
		timerTemplate: "[Unit]\nDescription=AiM: scheduled update check for %i\n\n" +
			"[Timer]\nPersistent=true\nRandomizedDelaySec=15min\n\n" +
			"[Install]\nWantedBy=timers.target\n",
	}
	for name, content := range units {
		if err := os.WriteFile(filepath.Join(s.UnitDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
