package proc_test

import (
	"os"
	"path/filepath"
	"testing"

	"appimage-manager/internal/proc"
)

const obsidian = "/home/jakub/Applications/obsidian/obsidian.AppImage"

// process adds a fake process to a fake /proc: exe is where its exe link
// points, environ its NUL-separated environment.
func process(t *testing.T, root, pid, exe, environ string) {
	t.Helper()
	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "exe")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environ"), []byte(environ), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunningWhenAProcessExecutesTheAppImage(t *testing.T) {
	root := t.TempDir()
	process(t, root, "4242", obsidian, "HOME=/home/jakub\x00")

	if !proc.Running(obsidian, root) {
		t.Fatal("want the AppImage's runtime process to count as running")
	}
}

func TestRunningWhenAProcessCameFromTheAppImagesMount(t *testing.T) {
	root := t.TempDir()
	process(t, root, "4243", "/tmp/.mount_obsidXyZ/obsidian", "HOME=/home/jakub\x00APPIMAGE="+obsidian+"\x00")

	if !proc.Running(obsidian, root) {
		t.Fatal("want a process with APPIMAGE set to the AppImage to count as running")
	}
}

func TestRunningWhenTheRunningFileWasAlreadyReplaced(t *testing.T) {
	root := t.TempDir()
	process(t, root, "4244", obsidian+" (deleted)", "")

	if !proc.Running(obsidian, root) {
		t.Fatal("want a process still running the replaced AppImage to count as running")
	}
}

func TestNotRunningWhenOnlyOtherAppsAre(t *testing.T) {
	root := t.TempDir()
	process(t, root, "100", "/usr/bin/konsole", "HOME=/home/jakub\x00")
	process(t, root, "101", "/home/jakub/Applications/obsidian-beta/obsidian-beta.AppImage", "APPIMAGE=/home/jakub/Applications/obsidian-beta/obsidian-beta.AppImage\x00")

	if proc.Running(obsidian, root) {
		t.Fatal("want obsidian not running")
	}
}

func TestRunningAReplacedFileOnlyWhenTheProcessUsesTheOldFile(t *testing.T) {
	root := t.TempDir()
	process(t, root, "200", obsidian, "")
	if proc.RunningReplaced(obsidian, root) {
		t.Fatal("a process running the current file doesn't need reopening")
	}

	process(t, root, "201", obsidian+" (deleted)", "")
	if !proc.RunningReplaced(obsidian, root) {
		t.Fatal("want a process still running the replaced file to need reopening")
	}
}
