// Package proc tells whether a Managed App is running.
package proc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// Running reports whether any process is running appImage: either the
// AppImage's runtime itself, or a process started from its mount (which
// the runtime marks with APPIMAGE=<path>). A process still running a file
// that has since been replaced counts too. procRoot is normally "/proc";
// processes AiM may not inspect are skipped.
func Running(appImage, procRoot string) bool {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false
	}
	marker := []byte("APPIMAGE=" + appImage)
	for _, e := range entries {
		dir := filepath.Join(procRoot, e.Name())
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil && strings.TrimSuffix(exe, " (deleted)") == appImage {
			return true
		}
		if environ, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
			for _, v := range bytes.Split(environ, []byte{0}) {
				if bytes.Equal(v, marker) {
					return true
				}
			}
		}
	}
	return false
}
