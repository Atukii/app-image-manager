// Command aim-gui is AiM's window and tray icon. It does everything by
// running aim (ADR 0001).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"appimage-manager/internal/gui"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	tray := flag.Bool("tray", false, "start with only the tray icon (as at login)")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *showVersion {
		fmt.Println("aim-gui", version)
		return
	}
	aim, err := findAim()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aim-gui:", err)
		os.Exit(1)
	}

	a := app.NewWithID("io.github.aim")
	ui := gui.New(a, gui.NewClient(gui.Exec{Path: aim}))
	ui.Reload()
	go func() { // keep Activity and the tray current
		for range time.Tick(5 * time.Minute) {
			ui.Reload()
		}
	}()
	if *tray {
		a.Run()
		return
	}
	ui.Window().Show()
	ui.Window().SetMaster()
	a.Run()
	_ = fyne.CurrentApp
}

// findAim finds the aim binary: next to aim-gui first, then on PATH.
func findAim() (string, error) {
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			sibling := filepath.Join(filepath.Dir(exe), "aim")
			if _, err := os.Stat(sibling); err == nil {
				return sibling, nil
			}
		}
	}
	path, err := exec.LookPath("aim")
	if err != nil {
		return "", fmt.Errorf("can't find the aim command next to aim-gui or on PATH; install aim first")
	}
	return path, nil
}
