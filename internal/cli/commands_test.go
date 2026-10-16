package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appimage-manager/internal/testkit"
)

// added is a fixture with Obsidian already added.
func added(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	if code, _, stderr := f.run("", "add", obsidianURL, "--yes"); code != 0 {
		t.Fatalf("setup: exit %d: %s", code, stderr)
	}
	return f
}

func TestListShowsAppsAsATable(t *testing.T) {
	f := added(t)

	code, stdout, _ := f.run("", "list")

	if code != 0 || !strings.Contains(stdout, "obsidian") || !strings.Contains(stdout, "v1.9.0") || !strings.Contains(stdout, "weekly") {
		t.Fatalf("exit %d, stdout:\n%s\nwant obsidian, v1.9.0 and weekly listed", code, stdout)
	}
}

func TestListAsJSON(t *testing.T) {
	f := added(t)

	_, stdout, _ := f.run("", "list", "--json")

	var got struct {
		Result []struct{ ID, Version string } `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || len(got.Result) != 1 || got.Result[0].Version != "v1.9.0" {
		t.Fatalf("stdout %q (%v); want obsidian v1.9.0 in the JSON result", stdout, err)
	}
}

func TestCheckAndUpdateInstallANewerRelease(t *testing.T) {
	for _, command := range []string{"check", "update"} {
		f := added(t)
		f.releases.Publish("v1.10.0", time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
			map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidianImage})

		code, stdout, stderr := f.run("", command, "obsidian")

		if code != 0 || !strings.Contains(stdout, "obsidian") || !strings.Contains(stdout, "v1.10.0") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want v1.10.0 installed and reported", command, code, stdout, stderr)
		}
	}
}

func TestSetPoll(t *testing.T) {
	f := added(t)

	code, _, stderr := f.run("", "set-poll", "obsidian", "daily")
	_, stdout, _ := f.run("", "list")

	if code != 0 || !strings.Contains(stdout, "daily") {
		t.Fatalf("exit %d, stderr %q, list:\n%s\nwant daily", code, stderr, stdout)
	}
}

func TestRemoveNeedsConfirmation(t *testing.T) {
	f := added(t)

	code, _, stderr := f.run("", "remove", "obsidian")
	if code != 2 || !strings.Contains(stderr, "--yes") || !f.exists("Applications/obsidian") {
		t.Fatalf("exit %d, stderr %q; want the Removal Plan refused without --yes", code, stderr)
	}

	if code, _, stderr := f.run("", "remove", "obsidian", "--yes"); code != 0 || f.exists("Applications/obsidian") {
		t.Fatalf("exit %d, stderr %q; want obsidian removed with --yes", code, stderr)
	}
}

func (f *fixture) read(rel string) string {
	data, _ := os.ReadFile(filepath.Join(f.home, rel))
	return string(data)
}

const entryPath = ".local/share/applications/md.obsidian.Obsidian.desktop"

func TestRollbackAndUnskip(t *testing.T) {
	f := added(t)
	f.releases.Publish("v1.10.0", time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidianImage})
	f.run("", "update", "obsidian")

	code, _, stderr := f.run("", "rollback", "obsidian")
	_, listed, _ := f.run("", "list")
	if code != 0 || !strings.Contains(listed, "v1.9.0") || !strings.Contains(listed, "v1.10.0 skipped") {
		t.Fatalf("exit %d, stderr %q, list:\n%s\nwant v1.9.0 back and v1.10.0 skipped", code, stderr, listed)
	}

	f.run("", "unskip", "obsidian")
	if _, stdout, _ := f.run("", "check", "obsidian"); !strings.Contains(stdout, "updated to v1.10.0") {
		t.Fatalf("check after unskip: %q; want v1.10.0 installed", stdout)
	}
}

func TestEditSetsFieldsArgumentsAndEnvironment(t *testing.T) {
	f := added(t)

	code, _, stderr := f.run("", "edit", "obsidian", "--set", "Name=My Notes", "--set", "Categories=Office;Utility;",
		"--arg", "--ozone-platform=wayland", "--env", "ELECTRON_OZONE_PLATFORM_HINT=wayland")

	entry := f.read(entryPath)
	if code != 0 || !strings.Contains(entry, "Name=My Notes\n") || !strings.Contains(entry, "Categories=Office;Utility;\n") ||
		!strings.Contains(entry, "Exec=env ELECTRON_OZONE_PLATFORM_HINT=wayland ") || !strings.Contains(entry, " --ozone-platform=wayland ") {
		t.Fatalf("exit %d, stderr %q, entry:\n%s", code, stderr, entry)
	}

	f.run("", "edit", "obsidian", "--reset", "Name", "--clear-args", "--clear-env")
	if entry := f.read(entryPath); !strings.Contains(entry, "Name=Obsidian\n") || strings.Contains(entry, "wayland") {
		t.Fatalf("after reset, entry:\n%s\nwant the embedded Name and no extra launch settings", entry)
	}
}

func TestUnmanageWithFlags(t *testing.T) {
	f := added(t)

	code, _, stderr := f.run("", "unmanage", "obsidian", "--layout", "standard", "--yes")

	if code != 0 || !f.exists("Applications/Obsidian.AppImage") || strings.Contains(f.read(entryPath), "X-AiM") {
		t.Fatalf("exit %d, stderr %q; want obsidian moved to the standard layout and unmanaged", code, stderr)
	}
}

func TestUninstallInsistsOnAConfigAnswer(t *testing.T) {
	f := added(t)

	code, _, stderr := f.run("", "uninstall", "--yes")
	if code != 2 || !strings.Contains(stderr, "--keep-config") {
		t.Fatalf("exit %d, stderr %q; want --yes alone refused, naming --keep-config", code, stderr)
	}

	if code, _, stderr := f.run("", "uninstall", "--yes", "--keep-config"); code != 0 || f.exists(".local/share/aim") {
		t.Fatalf("exit %d, stderr %q; want AiM uninstalled", code, stderr)
	}
}

func TestDoctorExitsNonZeroWhenItFindsProblems(t *testing.T) {
	f := added(t)
	if code, stdout, _ := f.run("", "doctor"); code != 0 || !strings.Contains(stdout, "No problems") {
		t.Fatalf("healthy: exit %d, %q", code, stdout)
	}
	os.Remove(filepath.Join(f.home, entryPath))

	code, stdout, _ := f.run("", "doctor")
	if code != 1 || !strings.Contains(stdout, "aim setup") {
		t.Fatalf("exit %d, stdout %q; want exit 1 suggesting aim setup", code, stdout)
	}

	f.run("", "setup")
	if code, _, _ := f.run("", "doctor"); code != 0 {
		t.Fatal("want no problems after aim setup")
	}
}

func TestAddFlags(t *testing.T) {
	f := newFixture(t)
	icon := filepath.Join(f.home, "my.svg")
	os.WriteFile(icon, []byte("<svg><!-- mine --></svg>"), 0o644)

	code, _, stderr := f.run("", "add", obsidianURL, "--yes", "--poll", "monthly", "--icon", icon)

	_, listed, _ := f.run("", "list")
	if code != 0 || !strings.Contains(listed, "monthly") || !strings.Contains(f.read("Applications/obsidian/obsidian.svg"), "mine") {
		t.Fatalf("exit %d, stderr %q, list:\n%s\nwant monthly checks and the user's icon", code, stderr, listed)
	}
}

// downloaded puts an Obsidian AppImage that matches no published Release
// in ~/Downloads.
func (f *fixture) downloaded(t *testing.T) string {
	t.Helper()
	older := testkit.AppImage{Files: map[string]string{
		"md.obsidian.Obsidian.desktop": obsidianImage.Files["md.obsidian.Obsidian.desktop"],
		"AppRun":                       "#!/bin/sh\n# 1.8.2\n",
	}}
	data, err := os.ReadFile(older.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.home, "Downloads", "Obsidian-1.8.2.AppImage")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o755)
	return path
}

func TestAdoptWithAVersionFlag(t *testing.T) {
	f := newFixture(t)
	path := f.downloaded(t)

	if code, _, stderr := f.run("", "adopt", path, obsidianURL, "--yes"); code != 2 || !strings.Contains(stderr, "--version") {
		t.Fatalf("exit %d, stderr %q; want the uncertain version refused, naming --version", code, stderr)
	}
	code, _, stderr := f.run("", "adopt", path, obsidianURL, "--version", "v1.8.2", "--yes")
	_, listed, _ := f.run("", "list")
	if code != 0 || !strings.Contains(listed, "v1.8.2") {
		t.Fatalf("exit %d, stderr %q, list:\n%s\nwant obsidian adopted at v1.8.2", code, stderr, listed)
	}
}

func TestScanLetsTheUserLeaveAnAppImageAlone(t *testing.T) {
	f := newFixture(t)
	path := f.downloaded(t)

	code, _, stderr := f.run("l\n", "scan")
	if code != 0 || !strings.Contains(stderr, path) {
		t.Fatalf("exit %d, stderr %q; want %s offered", code, stderr, path)
	}

	_, stdout, _ := f.run("", "scan", "--json")
	if strings.Contains(stdout, path) {
		t.Fatalf("got %q; want the ignored AppImage no longer offered", stdout)
	}
}

func TestScanCanManageWhatItFinds(t *testing.T) {
	f := newFixture(t)
	f.downloaded(t)

	// manage → Source URL → version → App ID → confirm
	code, _, stderr := f.run("m\n"+obsidianURL+"\nv1.8.2\n\n\n", "scan")

	_, listed, _ := f.run("", "list")
	if code != 0 || !strings.Contains(listed, "v1.8.2") {
		t.Fatalf("exit %d, stderr:\n%s\nlist:\n%s\nwant obsidian adopted through scan", code, stderr, listed)
	}
}

func TestSkipAndInspect(t *testing.T) {
	f := added(t)
	f.releases.Publish("v1.10.0", time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidianImage})

	code, stdout, stderr := f.run("", "skip", "obsidian")
	if code != 0 || !strings.Contains(stdout, "v1.10.0") {
		t.Fatalf("skip: exit %d, stdout %q, stderr %q; want v1.10.0 skipped", code, stdout, stderr)
	}
	if _, stdout, _ := f.run("", "check", "obsidian"); !strings.Contains(stdout, "skipped") {
		t.Fatalf("check after skip: %q; want v1.10.0 skipped", stdout)
	}

	withInfo := obsidianImage
	withInfo.UpdateInfo = "gh-releases-zsync|obsidianmd|obsidian-releases|latest|Obsidian-*.AppImage.zsync"
	code, stdout, _ = f.run("", "inspect", withInfo.Build(t))
	if code != 0 || !strings.Contains(stdout, "https://github.com/obsidianmd/obsidian-releases") {
		t.Fatalf("inspect: exit %d, stdout %q; want the suggested source", code, stdout)
	}
}

func TestTheAppIDPromptSaysWhatWasDownloaded(t *testing.T) {
	f := newFixture(t)

	_, _, stderr := f.run("notes\n", "add", obsidianURL)

	if !strings.Contains(stderr, "Obsidian-1.9.0.AppImage") || !strings.Contains(stderr, "verified") {
		t.Fatalf("stderr:\n%s\nwant the downloaded file named and said to be verified", stderr)
	}
}
