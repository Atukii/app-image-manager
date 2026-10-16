package testkit

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Systemd behaves like `systemctl --user` for the commands AiM uses: it
// only knows the unit files present in unitDir at its last daemon-reload, as
// real systemd does.
type Systemd struct {
	unitDir string
	loaded  []string
	// Enabled holds the enabled units.
	Enabled map[string]bool
	// FailEnable, when set, is returned by every enable command.
	FailEnable error
}

// NewSystemd returns a fake systemd using the units in unitDir.
func NewSystemd(unitDir string) *Systemd {
	return &Systemd{unitDir: unitDir, Enabled: map[string]bool{}}
}

// Run runs a systemctl command.
func (f *Systemd) Run(_ context.Context, args ...string) error {
	if len(args) == 0 || args[0] != "--user" {
		return fmt.Errorf("want --user, got %q", args)
	}
	switch cmd := strings.Join(args[1:], " "); {
	case cmd == "daemon-reload":
		f.loaded = nil
		entries, _ := os.ReadDir(f.unitDir)
		for _, e := range entries {
			f.loaded = append(f.loaded, e.Name())
		}
	case strings.HasPrefix(cmd, "enable --now "):
		unit := strings.TrimPrefix(cmd, "enable --now ")
		if f.FailEnable != nil {
			return f.FailEnable
		}
		if !slices.Contains(f.loaded, template(unit)) {
			return fmt.Errorf("Unit %s not found (no daemon-reload since it was written?)", unit)
		}
		f.Enabled[unit] = true
	case strings.HasPrefix(cmd, "disable --now "):
		unit := strings.TrimPrefix(cmd, "disable --now ")
		if !slices.Contains(f.loaded, template(unit)) {
			return fmt.Errorf("Failed to disable unit: Unit file %s does not exist", unit)
		}
		delete(f.Enabled, unit)
	default:
		return fmt.Errorf("unexpected systemctl command %q", cmd)
	}
	return nil
}

// template turns "aim-check@obsidian.timer" into "aim-check@.timer".
func template(unit string) string {
	at, dot := strings.Index(unit, "@"), strings.LastIndex(unit, ".")
	if at < 0 || dot < at {
		return unit
	}
	return unit[:at+1] + unit[dot:]
}
