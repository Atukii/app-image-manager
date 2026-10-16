// Package release works out the next semantic version from Conventional
// Commits (ADR 0010).
package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// header matches a Conventional Commit's first line: type(scope)!: subject.
var header = regexp.MustCompile(`^(\w+)(\([^)]*\))?(!)?: \S`)

type bump int

const (
	none bump = iota
	patch
	minor
	major
)

// Next returns the version that follows current (a "vX.Y.Z" tag, or "" for
// none yet) given the commit messages since it, or "" when nothing in them
// warrants a release. fix and perf are patches, feat is a minor, and "!" or
// a "BREAKING CHANGE:" footer is a major; before v1.0.0 a breaking change
// is a minor. The first release is v0.1.0.
func Next(current string, commits []string) string {
	b := none
	for _, msg := range commits {
		b = max(b, bumpFor(msg))
	}
	if b == none {
		return ""
	}
	if current == "" {
		return "v0.1.0"
	}
	var maj, min, pat int
	if _, err := fmt.Sscanf(strings.TrimPrefix(current, "v"), "%d.%d.%d", &maj, &min, &pat); err != nil {
		return ""
	}
	if b == major && maj == 0 {
		b = minor
	}
	switch b {
	case major:
		return "v" + strconv.Itoa(maj+1) + ".0.0"
	case minor:
		return fmt.Sprintf("v%d.%d.0", maj, min+1)
	}
	return fmt.Sprintf("v%d.%d.%d", maj, min, pat+1)
}

func bumpFor(msg string) bump {
	first, _, _ := strings.Cut(msg, "\n")
	m := header.FindStringSubmatch(first)
	if m == nil {
		return none
	}
	if m[3] == "!" || strings.Contains(msg, "\nBREAKING CHANGE:") || strings.Contains(msg, "\nBREAKING-CHANGE:") {
		return major
	}
	switch m[1] {
	case "feat":
		return minor
	case "fix", "perf":
		return patch
	}
	return none
}
