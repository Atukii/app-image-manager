// Command nextversion prints the next release's version, worked out from
// the Conventional Commits since the latest v* tag, or nothing when there
// is nothing to release (ADR 0010). CI runs it on every push to main.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"appimage-manager/internal/release"
)

func main() {
	latest := ""
	if out, err := exec.Command("git", "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*").Output(); err == nil {
		latest = strings.TrimSpace(string(out))
	}
	rng := "HEAD"
	if latest != "" {
		rng = latest + "..HEAD"
	}
	out, err := exec.Command("git", "log", rng, "--format=%B%x1e").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nextversion: reading commits:", err)
		os.Exit(1)
	}
	var commits []string
	for _, msg := range strings.Split(string(out), "\x1e") {
		if msg = strings.TrimSpace(msg); msg != "" {
			commits = append(commits, msg)
		}
	}
	if next := release.Next(latest, commits); next != "" {
		fmt.Println(next)
	}
}
