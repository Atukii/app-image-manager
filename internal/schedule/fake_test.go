package schedule_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
)

// fakeSystemd behaves like `systemctl --user` for the commands AiM uses: it
// only knows the unit files present in unitDir at its last daemon-reload, as
// real systemd does.
type fakeSystemd struct {
	unitDir string
	loaded  []string
	enabled map[string]bool
}

func newFakeSystemd(unitDir string) *fakeSystemd {
	return &fakeSystemd{unitDir: unitDir, enabled: map[string]bool{}}
}

func (f *fakeSystemd) Run(_ context.Context, args ...string) error {
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
		if !slices.Contains(f.loaded, template(unit)) {
			return fmt.Errorf("Unit %s not found (no daemon-reload since it was written?)", unit)
		}
		f.enabled[unit] = true
	case strings.HasPrefix(cmd, "disable --now "):
		unit := strings.TrimPrefix(cmd, "disable --now ")
		if !slices.Contains(f.loaded, template(unit)) {
			return fmt.Errorf("Failed to disable unit: Unit file %s does not exist", unit)
		}
		delete(f.enabled, unit)
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

func readFile(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
